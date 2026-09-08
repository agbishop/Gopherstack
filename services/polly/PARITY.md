---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: polly
sdk_module: aws-sdk-go-v2/service/polly@v1.60.4   # version audited against
last_audit_commit: eb5faf60f                      # HEAD when this manifest was written
last_audit_date: 2026-09-04
overall: A            # two real bugs found and fixed this pass (StartSpeechSynthesisStream accepted non-generative Engine values; StartSpeechSynthesisTask shared SynthesizeSpeech's wider mp3/ogg_vorbis SampleRate set)
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  SynthesizeSpeech: {wire: ok, errors: ok, state: ok, persist: n/a, note: "OutputFormat coverage complete (ogg_opus/mulaw/alaw); full op-specific error taxonomy; SSML well-formedness now validated (InvalidSsmlException)"}
  StartSpeechSynthesisStream: {wire: ok, errors: ok, state: n/a, persist: n/a, note: "FIXED 2026-09-04: Engine was validated against SynthesizeSpeech's full 4-value set (standard/neural/long-form/generative) and defaulted an unset value to standard, but api_op_StartSpeechSynthesisStream.go's Engine doc comment says 'Currently, only the generative engine is supported' -- any other value (including unset) is now rejected as ValidationException before the shared SynthesizeSpeech validation path runs. OutputFormat=json (the doc's other stated restriction, 'does not support JSON speech marks') was already rejected, coincidentally: this op never reads SpeechMarkTypes from headers, so validateSpeechMarks' 'json requires SpeechMarkTypes' rule always fires for it -- confirmed, not changed. Error taxonomy (from prior pass) remains correct: every client validation failure remaps to the generic ValidationException, matching the real deserializer's error switch (ServiceFailureException/ServiceQuotaExceededException/ThrottlingException/ValidationException). ServiceQuotaExceededException/ThrottlingException remain unimplemented -- see Notes."}
  StartSpeechSynthesisTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED: removed fabricated SnsRoleArn request/response field (not a real Polly API field -- see Notes); added real OutputS3KeyPrefix request field wired into OutputUri; added S3 bucket/key and SNS topic ARN format validation; SSML-vs-plain-text length limit now correctly differentiated (100000 billed / 200000 total, was flat 100000 for both). FIXED 2026-09-04: SampleRate for mp3/ogg_vorbis now correctly narrower than SynthesizeSpeech's (8000/16000/22050/24000 only, no 44100/48000) per this op's own SampleRate doc comment, which was previously sharing SynthesizeSpeech's 6-value set via the common validateOptions helper."}
  GetSpeechSynthesisTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED: added InvalidTaskIdException for syntactically invalid (non-UUID) TaskId, distinct from SynthesisTaskNotFoundException for a well-formed-but-unknown one -- both are real, separately-modeled exceptions for this op"}
  ListSpeechSynthesisTasks: {wire: ok, errors: ok, state: ok, persist: ok, note: "MaxResults out-of-range left generic (unlisted in the real service model -- confirmed via deserializer's error switch, which lists only InvalidNextTokenException/ServiceFailureException)"}
  PutLexicon: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED: implemented LexiconSizeExceededException (>40000 chars), MaxLexemeLengthExceededException (>100 char <phoneme>/<alias> replacement), MaxLexiconsNumberExceededException (>100 lexicons/account), UnsupportedPlsAlphabetException (alphabet not ipa/x-sampa), UnsupportedPlsLanguageException (xml:lang outside the 42-value LanguageCode enum) -- all quota numbers sourced from docs.aws.amazon.com/polly/latest/dg/limits.html#limits-lexicons"}
  GetLexicon: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteLexicon: {wire: ok, errors: ok, state: ok, persist: ok}
  ListLexicons: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-20: each Lexicons[] entry was a flat map (Name+Alphabet+LanguageCode+LastModified+LexemesCount+LexiconArn+Size as siblings), not the real LexiconDescription shape (Name sibling of a nested Attributes object). Real SDK's deserializeDocumentLexiconDescription only reads top-level Name/Attributes keys, so every attribute field was silently dropped by a real client. Also removed a stray Name key from the shared LexiconAttributes payload (GetLexicon's LexiconAttributes root member and ListLexicons' nested Attributes both had it; real LexiconAttributes has no Name field)."}
  DescribeVoices: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED: built-in voice catalogue now covers all 106 VoiceId enum values in the pinned SDK (was ~87/106); every existing entry's SupportedEngines re-verified against docs.aws.amazon.com/polly/latest/dg/voicelist.html and corrected where wrong (many voices were missing their generative-engine support, a few had extra/missing standard or neural support -- see Notes). Still no MaxResults/NextToken pagination -- confirmed correct AWS behavior (single-page response is valid), not changed."}
families:
  lexicon: {status: ok, note: "Put/Get/List/Delete verified against restjson1 paths and PutLexicon/GetLexicon shapes; quota/PLS-schema validation field-diffed against limits.html and the real UnsupportedPlsAlphabetException doc string; persistence round-trips (store.Table). 2026-08-20: fixed ListLexicons' Attributes nesting (see ListLexicons op note) and confirmed by real-SDK round trip (wire_sdk_roundtrip_test.go)"}
  synthesisTask: {status: ok, note: "Start/Get/List verified; SnsRoleArn (fabricated) removed, OutputS3KeyPrefix (real, previously missing) added and wired into OutputUri; S3/SNS format validation added; lifecycle advance-on-poll unchanged and correct; persist round-trips including the new OutputS3KeyPrefix field"}
  synthesizeSpeech: {status: ok, note: "REST payload response verified: Content-Type set from OutputFormat, X-Amzn-RequestCharacters header present; speech-mark json-stream verified; SSML well-formedness (must be valid XML wrapped in <speak>) now enforced for both SynthesizeSpeech and StartSpeechSynthesisTask via one shared validateSSML, and for StartSpeechSynthesisStream via SynthesizeSpeech's shared validateOptions"}
  voices: {status: ok, note: "filter logic (Engine/Gender/LanguageCode/IncludeAdditionalLanguageCodes) verified against real DescribeVoicesInput/Voice shape; full 106-voice catalogue field-diffed against the AWS voicelist.html table and the pinned SDK's VoiceId enum -- every voice's LanguageCode/Gender/SupportedEngines cross-checked"}
gaps: []
deferred: []
leaks: {status: clean, note: "no goroutines/timers; task lifecycle advances synchronously on each Get/List poll (b.mu-guarded), no background janitor to leak. Tag* removal deleted the last map keyed independently of store.Table (b.tags) with no replacement -- one fewer thing that could ghost-row after delete."}
---

## Notes

Freeform: AWS-behavior specifics worth remembering, and any "looks-wrong-but-correct" traps
so the next auditor doesn't re-flag them.

- **Protocol**: restjson1. Response bodies for the management ops (Lexicon*, SpeechSynthesisTask*)
  are `{"...": ...}` JSON envelopes; `SynthesizeSpeech`/`StartSpeechSynthesisStream` are raw
  bodies (audio bytes / eventstream frames), not JSON envelopes -- this is correct AWS behavior,
  not a missing envelope bug.

- **SynthesisTaskNotFoundException is HTTP 400, not 404.** Confirmed directly from
  `botocore/data/polly/2016-06-10/service-2.json`: `'error': {'httpStatusCode': 400}`. This is
  unusual for a "NotFound"-named exception (compare `LexiconNotFoundException`, which genuinely is
  404) but is the real, documented AWS behavior -- do not "fix" it back to 404 in a future pass.

- **OutputFormat has 7 real values**: `json, mp3, ogg_opus, ogg_vorbis, pcm, mulaw, alaw`, all
  covered with correct Content-Type and SampleRate constraints.

- **PCM's synthetic bytes are wrapped in a RIFF/WAV header even though AWS's real `pcm` output is
  headerless raw signed-16-bit little-endian samples** (Content-Type `audio/pcm`, not
  `audio/wav`). This is a pre-existing minor inconsistency in the mock audio byte content, not
  the wire shape (Content-Type/RequestCharacters headers are correct). Per prior audit scope
  ("mock audio bytes are acceptable"), left unchanged -- do not "fix" without re-confirming scope,
  since every existing PCM test (`bodyMagic: []byte("RIFF")`, WAV-header sample-rate byte offsets)
  depends on the WAV wrapper.

- **Error taxonomy is now complete per-op**, verified directly against each operation's
  `awsRestjson1_deserializeOpError<Op>` switch in `aws-sdk-go-v2/service/polly/deserializers.go`
  (the modeled error list, not guessed): `SynthesizeSpeech`/`StartSpeechSynthesisTask` share
  {EngineNotSupported, InvalidSampleRate, InvalidSsml, LanguageNotSupported, LexiconNotFound,
  MarksNotSupportedForFormat, ServiceFailure, SsmlMarksNotSupportedForTextType,
  TextLengthExceeded} (+S3Bucket/S3Key/SnsTopicArn for the task op only); `PutLexicon` has
  {InvalidLexicon, LexiconSizeExceeded, MaxLexemeLengthExceeded, MaxLexiconsNumberExceeded,
  ServiceFailure, UnsupportedPlsAlphabet, UnsupportedPlsLanguage}; `GetSpeechSynthesisTask` has
  {InvalidTaskId, ServiceFailure, SynthesisTaskNotFound}; `List*`/`DescribeVoices` have only
  {InvalidNextToken, ServiceFailure}; `StartSpeechSynthesisStream` has only {ServiceFailure,
  ServiceQuotaExceeded, Throttling, Validation} -- notably NOT the op-specific names above, so
  every validation failure on that op remaps to the generic `ValidationException` (see
  `ErrStreamValidation`'s doc comment in errors.go). Checks with no modeled exception at all
  (invalid Engine/OutputFormat/TextType enum values, LexiconNames-count-exceeded, unknown VoiceId
  for SynthesizeSpeech/StartSpeechSynthesisTask, MaxResults out of range, lexicon
  name-format violation) intentionally stay on the generic `ErrValidation` →
  `InvalidParameterValueException` fallback for those two ops -- this is standard AWS behavior
  for unlisted/unmodeled validation errors (returned via the SDK's generic
  `smithy.GenericAPIError` path), not a gap.

- **`checkVoiceSupport` (speech.go)** distinguishes "voice ID doesn't exist" (generic
  `ErrValidation`) from "voice exists but doesn't support this engine"
  (`EngineNotSupportedException`) from "voice exists, engine ok, but doesn't speak this language"
  (`LanguageNotSupportedException`). All three are real, separately-named AWS exceptions with
  distinct meanings per the service model's documentation strings.

- **RouteMatcher / dispatch use the same `parseRoute` helper** — `Handler()`'s dispatcher and
  `RouteMatcher()` both call `parseRoute(method, path)`; there is no separate/duplicate routing
  table, so unit tests calling `h.Handler()(c)` directly do NOT bypass real routing logic here.
  Paths/methods verified against `aws-sdk-go-v2/service/polly/serializers.go`
  (`/v1/lexicons/{Name}` PUT/GET/DELETE, `/v1/lexicons` GET, `/v1/voices` GET,
  `/v1/synthesisTasks` POST/GET, `/v1/synthesisTasks/{TaskId}` GET, `/v1/synthesisStream` POST,
  `/v1/speech` POST) -- this list is now exhaustive; there is no `/v1/tags/{arn}` route.

- **Tagging surface removed.** `TagResource`/`UntagResource`/`ListTagsForResource` and the
  `/v1/tags/{arn}` routes were gopherstack-invented functionality with zero basis in the real
  Amazon Polly API (confirmed: no `api_op_TagResource.go` et al. in `aws-sdk-go-v2/service/polly`,
  and `service-2.json`'s operation list omits them entirely). The prior audit flagged this as a
  gap needing a decision ("confirm intentional and document, or remove") but left it in place;
  this pass removed it for true parity -- `tags.go`/`tags_test.go` deleted, the `b.tags` map and
  `TaskARN`/`taskARN` helpers deleted, the three ops dropped from
  `GetSupportedOperations`/routing/dispatch, `Tag` struct removed from models.go, and
  `backendSnapshot.Tags` dropped (snapshot version bumped 1→2, discarding any snapshot with tag
  data on restore -- acceptable since the feature no longer exists). `sdk_completeness_test.go`
  still passes: `sdkcheck.CheckCompleteness` only verifies gopherstack doesn't have *fewer* ops
  than the SDK client surface, never more, so this was never a completeness-test dependency.

- **`SnsRoleArn` was a fabricated field, now removed.** Real Polly's
  `StartSpeechSynthesisTaskInput`/`SynthesisTask` (request and response) have no `SnsRoleArn`
  field at all -- confirmed directly in `aws-sdk-go-v2/service/polly/api_op_StartSpeechSynthesisTask.go`.
  Only `SnsTopicArn` is real. The field was silently invented (possibly confused with a different
  AWS service that does have an SNS role ARN parameter) and has been deleted from
  `startTaskInput`/`taskOutput`/`SpeechSynthesisTask`/the backend method signature. Real
  `OutputS3KeyPrefix`, which existed on the real request type but was never read by gopherstack's
  handler (silently dropped), is now wired through end-to-end and woven into the constructed
  `OutputUri`.

- **Voice catalogue is exhaustively field-diffed.** All 106 `VoiceId` enum values from the pinned
  SDK (`aws-sdk-go-v2/service/polly/types`) are present. Three voices AWS's live documentation page
  lists (Patrick, Alba, Raúl) are intentionally excluded: they are not part of the pinned SDK's
  `VoiceId` enum (a newer AWS addition unreleased at pin time), so accepting them would let this
  backend respond to a `VoiceId` no real client built against `v1.57.5` could ever send. Every
  voice's `LanguageCode`/`Gender`/`SupportedEngines` was cross-checked against
  `docs.aws.amazon.com/polly/latest/dg/voicelist.html`'s table (fetched live during this pass, not
  reconstructed from memory) -- this caught and fixed several pre-existing `SupportedEngines`
  errors beyond the 19 missing voices, most commonly a voice missing `generative` engine support it
  actually has (e.g. Lisa, Laura, Olivia, Kajal, Niamh, Aria, Ayanda, Remi, Isabelle, Gabrielle,
  Liam, Vicki, Daniel, Hannah, Ola, Camila, Lucia, Sergio, Mia, Lupe, Pedro, Seoyeon) plus a few
  outright-wrong entries (Justin had `standard` it doesn't support; Kevin was missing `standard` it
  does support; Joanna/Matthew/Ruth/Stephen had the wrong long-form/generative mix; Lotte had
  `neural` it doesn't support). Bilingual voices (Aditi/Kajal: en-IN+hi-IN; Hala/Zayd: ar-AE+arb)
  use `AdditionalLanguageCodes` per `docs.aws.amazon.com/polly/latest/dg/bilingual-voices.html`,
  which confirms Hala/Zayd are Amazon Polly's only other fully bilingual voices besides Aditi/Kajal.

- **SSML well-formedness validation (`validateSSML` in speech.go)** requires TextType=ssml input
  to be well-formed XML with exactly one root element named `speak` (checked via `encoding/xml`
  tokenization, not a regex) -- unwrapped plain text or malformed markup now returns
  `InvalidSsmlException`. This is shared by `SynthesizeSpeech`, `StartSpeechSynthesisTask` (both
  call the common `validateOptions`), and `StartSpeechSynthesisStream` (calls `SynthesizeSpeech`
  internally, then remaps any resulting error including this one to `ValidationException` per the
  stream op's real error taxonomy above).

- **Lexicon quotas** (`docs.aws.amazon.com/polly/latest/dg/limits.html#limits-lexicons`, fetched
  live): lexicon content ≤40,000 characters, ≤100 lexicons per account, ≤100 characters per
  `<phoneme>`/`<alias>` replacement, lexicon name ≤20 alphanumeric characters (already correct
  pre-pass). `MaxLexiconsNumberExceededException` only fires for a genuinely new lexicon name;
  overwriting an existing one never counts against the quota (matches real `PutLexicon` semantics:
  "If a lexicon with the same name already exists ... it is overwritten").

- **StartSpeechSynthesisTask text limits corrected to differentiate TextType**:
  100,000 billed characters (plain text) vs 200,000 total characters (SSML, markup not billed),
  per `docs.aws.amazon.com/polly/latest/dg/limits.html#limits-long`. Was previously a flat 100,000
  regardless of TextType, incorrectly rejecting valid SSML requests between 100,001 and 200,000
  characters.

- **`GetSpeechSynthesisTask` now validates TaskId format.** A syntactically invalid (non-UUID)
  TaskId returns `InvalidTaskIdException`; a well-formed UUID that doesn't match any task returns
  `SynthesisTaskNotFoundException`. Confirmed reachable server-side behavior (not merely client-side
  SDK validation): `aws-sdk-go-v2/service/polly/validators.go` only checks `TaskId` is non-nil, not
  its format, so a real HTTP client (not just the Go SDK) can trigger this server-side. Task IDs are
  UUIDs (see `uuid.NewString()` in `StartSpeechSynthesisTask`).

- **Not implemented, and not claimed as fixed**: `ServiceQuotaExceededException`/
  `ThrottlingException` for `StartSpeechSynthesisStream` require genuine request-rate/quota
  simulation (a different kind of feature entirely, unrelated to input-validation taxonomy) and
  were out of scope for this pass; the `ValidationException` remapping for actual client input
  errors is complete and correct on its own.

## polly (this session, 2026-08-20)

Wrapper-key / nested-shape wire-parity sweep. Protocol confirmed restjson1 (`awsRestjson1_*`
prefix in `deserializers.go`; `sdkshape.sh polly` agrees). All 10 ops in
`GetSupportedOperations` match the 10 `api_op_*.go` files in the pinned SDK
(`aws-sdk-go-v2/service/polly@v1.60.4`) -- no missing, no extra.

**Provenance of the prior stamp**: `last_audit_commit: b0d0cfe0` dates to 2026-07-13
(`git show -s --format=%ad b0d0cfe0`), but the file's `last_audit_date` read `2026-07-23` -- a
10-day gap with the commit predating the date. Traced via `git log -p -- services/polly/PARITY.md`:
`d1235ad54` (2026-07-13) did the real audit work and set both fields to 07-13; a later commit
`d39bf33e4` ("Chore/parity upgrade") bumped `sdk_module` from v1.57.5 to v1.60.4 and pushed
`last_audit_date` to 07-23 *without* changing `last_audit_commit` or re-verifying wire shapes --
a version-pin-only touch, not a re-audit. Separately, `fb80d66cd` (2026-08-17, after both stamped
dates) touched `services/polly/{lexicons,persistence,speech,speech_synthesis_tasks,store,voices}.go`
-- read in full: purely `sync.RWMutex` -> `lockmetrics.RWMutex` migration (adding op-name labels
to `Lock`/`RLock` calls), zero wire-shape changes. So real code drift since the last true audit
was lock-instrumentation only; my own independent sweep (below) is what found the real bug.

**Ops swept**: all 10 -- DeleteLexicon, DescribeVoices, GetLexicon, GetSpeechSynthesisTask,
ListLexicons, ListSpeechSynthesisTasks, PutLexicon, StartSpeechSynthesisStream,
StartSpeechSynthesisTask, SynthesizeSpeech.

**Payload-bound vs JSON-document per op**:
- `SynthesizeSpeech`: **payload-bound**. Confirmed by reading `HandleDeserialize` directly:
  it calls `awsRestjson1_deserializeOpHttpBindingsSynthesizeSpeechOutput` (Content-Type and
  `x-amzn-RequestCharacters` headers) THEN calls `awsRestjson1_deserializeOpDocumentSynthesizeSpeechOutput`
  -- but that function's body is `v.AudioStream = body; return nil` (deserializers.go:1458), i.e.
  it is called but does NOT decode JSON; it just assigns the raw body reader. This is exactly the
  `gopherstack-cnhp` trap the brief warned about -- the OpDocument helper name looks like a JSON
  decode but isn't one for this op.
- `StartSpeechSynthesisStream`: request is **header-bound** (Engine/LanguageCode/LexiconNames/
  OutputFormat/SampleRate/VoiceId all HTTP headers, no JSON body); response is an eventstream, not
  a JSON document.
- `DescribeVoices`, `GetLexicon`, `ListLexicons`, `PutLexicon` (void), `DeleteLexicon` (void),
  `StartSpeechSynthesisTask`, `GetSpeechSynthesisTask`, `ListSpeechSynthesisTasks`: ordinary JSON
  response bodies.

**SynthesizeSpeech header bindings**: emitted correctly. gopherstack sets `Content-Type` from
`result.ContentType` and `X-Amzn-Requestcharacters` via `strconv.Itoa(result.RequestCharacters)`
(handler.go). The real serializer reads headers `Content-Type` and `x-amzn-RequestCharacters`
(deserializers.go, `awsRestjson1_deserializeOpHttpBindingsSynthesizeSpeechOutput`); Go's
`http.Header.Set/Get` canonicalizes both spellings to the identical wire form
(`textproto.CanonicalMIMEHeaderKey`), so `X-Amzn-Requestcharacters` == `X-Amzn-RequestCharacters`
on the wire -- not a bug, verified by reading `net/textproto`'s canonicalization rule, not assumed.

**GetLexicon vs ListLexicons nesting -- BUG FOUND AND FIXED.** `GetLexicon`'s two root members
(`Lexicon{Content,Name}` + sibling `LexiconAttributes{Alphabet,LanguageCode,LastModified,
LexemesCount,LexiconArn,Size}`) were already correct. But `ListLexicons`' `Lexicons[]` entries were
built as a FLAT map (Name plus all six attribute fields as siblings) instead of the real
`LexiconDescription{Name, Attributes: LexiconAttributes{...}}` shape -- confirmed directly against
`aws-sdk-go-v2/service/polly/deserializers.go:3639`
(`awsRestjson1_deserializeDocumentLexiconDescription` reads only top-level `"Attributes"` and
`"Name"` keys per item; any other key at that level silently falls into its `default: _, _ = key,
value` no-op). A real SDK client parsing gopherstack's old response therefore got
`LexiconDescription.Attributes == nil` for every lexicon in the list -- every attribute field
silently dropped. Fixed in `services/polly/handler.go`'s `listLexicons` (nests each entry's
attribute fields under an `"Attributes"` key) and its shared `lexiconAttributes` helper (dropped a
stray `"Name"` key that doesn't belong in the real `LexiconAttributes` shape at all, for both
`GetLexicon`'s root member and the new nested `Attributes`). Proven end-to-end with a new real-SDK
round-trip test, `TestListLexicons_AttributesNested_SDKRoundTrip`
(`services/polly/wire_sdk_roundtrip_test.go`) -- fails (`Attributes` nil) against the pre-fix code,
passes against the fix. Hand-revert (`cp` the pre-fix `handler.go` back in, `md5sum`-verified round
trip both ways) reproduces the exact failure and confirms the fix is what closes it.

**SynthesisTask item shape**: identical across `StartSpeechSynthesisTask`/`GetSpeechSynthesisTask`/
`ListSpeechSynthesisTasks` -- all three build their `SynthesisTask` JSON via the single shared
`buildTaskOutput()` / `taskOutput` struct in handler.go. Field-diffed against the real
`types.SynthesisTask` struct (types/types.go:166): 15 fields on both sides, one-to-one
(CreationTime, Engine, LanguageCode, LexiconNames, OutputFormat, OutputUri, RequestCharacters,
SampleRate, SnsTopicArn, SpeechMarkTypes, TaskId, TaskStatus, TaskStatusReason, TextType, VoiceId)
-- no fabricated or missing members. `CreationTime` epoch-seconds encoding
(`float64(t.UnixMilli())/1000`) matches the real deserializer's `ParseEpochSeconds` JSON-number
handling (deserializers.go, `awsRestjson1_deserializeDocumentSynthesisTask`).

**Enums checked both directions** (every SDK value representable by gopherstack, AND every
constant gopherstack emits is a real SDK value):
- `Engine` (4: standard/neural/long-form/generative) -- exact match.
- `Gender` (2: Female/Male) -- exact match.
- `OutputFormat` (7: json/mp3/ogg_opus/ogg_vorbis/pcm/mulaw/alaw) -- exact match
  (`validOutputFormats()` in speech.go).
- `SpeechMarkType` (4: sentence/ssml/viseme/word) -- exact match (`validSpeechMarkTypes()`).
- `TaskStatus` (4: scheduled/inProgress/completed/failed) -- exact match (`validTaskStatuses()`).
- `TextType` (2: text/ssml) -- exact match (`validTextTypes()`).
- `VoiceId` (106 values) -- diffed programmatically: `awk` over the pinned SDK's
  `(VoiceId) Values()` vs every `ID: "..."` literal in `services/polly/voices.go`; `diff` between
  the two sorted 106-line lists is empty. Exact match both directions, confirming the prior
  audit's claim of full VoiceId coverage.
- `LanguageCode` (42 values): no explicit whitelist check exists in gopherstack (DescribeVoices'
  LanguageCode filter just string-matches, an unmatched code returns an empty list rather than an
  error) -- confirmed this is correct, not a gap: `DescribeVoices`' real error switch
  (deserializers.go, `awsRestjson1_deserializeOpErrorDescribeVoices`) models only
  `InvalidNextTokenException`/`ServiceFailureException`, no LanguageCode-specific exception exists
  server-side to enforce against.

**Error taxonomy re-verified per op** (all 9 non-void ops' `awsRestjson1_deserializeOpError<Op>`
switches read directly, not from memory): matches `onceErrorTable` in handler.go exactly,
including `SynthesisTaskNotFoundException` at HTTP 400 (not 404) and `StartSpeechSynthesisStream`'s
generic-only taxonomy (ServiceFailure/ServiceQuotaExceeded/Throttling/Validation, no op-specific
names). Confirms the prior audit's documented taxonomy without changes.

**StartSpeechSynthesisStream header bindings** (request side): `X-Amzn-Engine`,
`X-Amzn-Languagecode`, `X-Amzn-Lexiconnames`, `X-Amzn-Outputformat`, `X-Amzn-Samplerate`,
`X-Amzn-Voiceid` -- all 6 match `serializeOpHttpBindingsStartSpeechSynthesisStreamInput`
(serializers.go:603) exactly.

**Families clean**: synthesisTask, synthesizeSpeech, voices (all re-verified, zero deviations).
lexicon: one real bug found and fixed (see above).

**Structurally unverifiable**: the synthetic audio byte content itself (PCM/MP3/OGG/mulaw/alaw
payloads) is mock data by design across this codebase -- headers/Content-Type/RequestCharacters
are the verifiable wire contract for `SynthesizeSpeech`, and those are correct. Not re-litigating
the pre-existing, explicitly-scoped decision to keep PCM's RIFF/WAV wrapper (documented in the
Notes above).

**Gaps disclosed, not fixed** (unchanged from prior audit, still out of scope for a wire-parity
sweep): `ServiceQuotaExceededException`/`ThrottlingException` for `StartSpeechSynthesisStream`
require real rate/quota simulation, not a shape fix.

**Existing wrong-key tests corrected**: none needed correction -- the pre-existing
`lexicons_test.go` list-route assertion only does a `find: '"Name":"alpha"'` substring check,
which is a true substring of both the old flat shape and the new nested shape, so it neither
caught nor masked the bug. No test asserted the (wrong) flat shape as intentional.

**Brief accuracy**: everything in the brief matched the pinned SDK -- no fabricated hints found
this session.

**Gates** (from `services/polly/`):
```
go build ./services/polly/...     -> ok
go vet ./services/polly/...       -> clean
go fix -diff ./services/polly/... -> empty
gofmt -l services/polly/          -> empty
go test -race ./services/polly/... -> ok (fresh, non-cached)
golangci-lint run ./services/polly/... -> 0 issues
```
`git status --short` at session end shows only `services/polly/handler.go` (modified) and
`services/polly/wire_sdk_roundtrip_test.go` (new) as my changes; other dirty files
(`services/applicationautoscaling/*`) belong to a concurrent, unrelated sweep session and were not
touched by me.

## polly (this session, 2026-09-04, gopherstack-22s)

Parity-bug hunt against the bug patterns list (missing delete preconditions, enum/input
validation, inert config, ghost rows, resource leaks, discarded parameters, fabricated
values, stale cache). Re-verified the prior audit's claims independently rather than trusting
them: error taxonomy for all 10 ops re-derived from `deserializers.go`'s
`awsRestjson1_deserializeOpError<Op>` switches (exact match to `errors.go`/`handler.go`'s
`onceErrorTable`); every exception's `httpStatusCode` re-derived from `botocore`'s
`polly` service model (`ServiceModel.shape_for(name).metadata['error']`, since the Go SDK
itself doesn't carry the trait) -- all 23 statuses match `onceErrorTable` exactly, including the
two non-default ones (`LexiconNotFoundException` 404, `SynthesisTaskNotFoundException` 400);
`VoiceId` (106) and `LanguageCode` (42) enum coverage re-diffed programmatically against
`types/enums.go` -- both exact matches, zero missing/extra. Lexicon storage confirmed
`store.Table`-backed (structurally immune to ghost-row-after-delete). Sample-rate-per-format and
Content-Type-per-format tables in `speech.go` cross-checked line-by-line against
`api_op_SynthesizeSpeech.go`'s doc comment -- exact match.

**BUG FOUND AND FIXED: `StartSpeechSynthesisStream` accepted every `Engine` value.**
`api_op_StartSpeechSynthesisStream.go`'s `Engine` field doc comment: "Specifies the engine for
Amazon Polly to use when processing input text for speech synthesis. **Currently, only the
generative engine is supported.** If you specify a voice that the selected engine doesn't
support, Amazon Polly returns an error." This is a real, per-operation-specific restriction: the
same struct's `SynthesizeSpeech`/`StartSpeechSynthesisTask` inputs document all 4 Engine values as
valid, and an unset one there defaults to standard. gopherstack's `startSpeechSynthesisStream`
built its `SynthesisOptions.Engine` from the raw `X-Amzn-Engine` header and then fed it straight
into the shared `SynthesizeSpeech`-path `validateOptions`, which accepts all 4 values -- so a
stream request with Engine=standard/neural/long-form/unset for a voice that supports that engine
synthesized normally instead of failing. Fixed in `handler.go`'s `startSpeechSynthesisStream`:
reject any Engine other than exactly `"generative"` up front, before the shared validation path,
returning `ErrStreamValidation` (this op's real, sole client-error type per its deserializer's
error switch -- see the existing `EngineNotSupportedException`-vs-`ValidationException` taxonomy
note above). Not client-side-SDK-enforced (`validators.go`'s
`validateOpStartSpeechSynthesisStreamInput` only checks Engine is non-empty, not which value), so
this is reachable by any real HTTP client, not just a hand-rolled one -- a genuine server-side gap,
not a documentation nuance. New regression test
`TestStartSpeechSynthesisStreamRequiresGenerativeEngine` (`speech_test.go`) covers
standard/neural/long-form/unset, using voices (Joanna, Danielle) that legitimately support each
rejected engine so the only possible cause of a 400 is the new Engine gate, not
`EngineNotSupportedException`. Verified failing pre-fix (all 4 subtests returned HTTP 200 instead
of 400 when the fix's 9 added lines were removed and restored via `cp`, diff-counted before restore)
and passing post-fix.

**Also checked, not a bug (documented for the next auditor):** the same doc comment's other
stated restriction -- "Currently, Amazon Polly does not support JSON speech marks" -- is already
correctly rejected, coincidentally: `StartSpeechSynthesisStreamInput` has no `SpeechMarkTypes`
field, so gopherstack's header-parsing never populates `SynthesisOptions.SpeechMarkTypes` for this
op, meaning `validateSpeechMarks`'s existing "OutputFormat=json requires non-empty
SpeechMarkTypes" rule always fires for OutputFormat=json on this path. Net behavior is already
correct; no code change made for this half of the doc comment.

**Also checked, not a bug:** `listLexicons` reads and honors a `MaxResults` query parameter, but
the real `ListLexiconsInput` (`api_op_ListLexicons.go`) has no `MaxResults` field at all (only
`NextToken`) -- a genuine SDK client can never send it, so this is unreachable-but-harmless code,
not a wire bug; confirmed the default page size (100) can never truncate a real response since
`maxLexiconsPerAccount` is also 100, so real client behavior is unaffected either way. Left
unchanged (removing it is a separate, non-bug cleanup decision, not a parity fix).

**Also checked, not a bug:** `SpeechSynthesisTask.polls` (`models.go`) is incremented in
`advanceTask` (`speech_synthesis_tasks.go`) but never read anywhere. Dead state, not a wire or
persistence bug (nothing observable depends on it) -- flagged for a future cleanup pass, not fixed
here since it's out of this campaign's "AWS parity bug" scope.

**SECOND BUG FOUND AND FIXED: `StartSpeechSynthesisTask` shared `SynthesizeSpeech`'s wider
mp3/ogg_vorbis `SampleRate` set.** Both ops' `SampleRate` doc comments were read in full (not
grepped) since they're line-wrapped across several `//` lines each.
`api_op_SynthesizeSpeech.go`: "The valid values for mp3 and ogg_vorbis are \"8000\", \"16000\",
\"22050\", \"24000\", \"44100\" and \"48000\"." `api_op_StartSpeechSynthesisTask.go`: "The valid
values for mp3 and ogg_vorbis are \"8000\", \"16000\", \"22050\", and \"24000\"." -- 4 values, not
6; 44100/48000 are absent. Every other format's valid set (pcm/ogg_opus/mulaw/alaw) is worded
identically in both files, so this narrowing is specific to mp3/ogg_vorbis on the task op. Both
ops funneled through one shared `validateOptions`/`validSampleRate` in `speech.go`, so
`StartSpeechSynthesisTask` was wrongly accepting SampleRate=44100/48000 for mp3/ogg_vorbis (a real
client would get an unexpected 200 where AWS returns `InvalidSampleRateException`, which this op's
error switch does model -- confirmed reachable, not just a docs nuance). Fixed by adding a
`forTask bool` parameter to `validateOptions`/`validSampleRate` (`speech.go`), threaded from each
call site: `SynthesizeSpeech` passes `false` (unchanged 6-value behavior),
`StartSpeechSynthesisTask` passes `true` (new 4-value behavior for mp3/ogg_vorbis only). New
regression test `TestStartSpeechSynthesisTaskSampleRateNarrowerThanSynthesizeSpeech`
(`speech_synthesis_tasks_test.go`) asserts both halves of the contrast per case: the same
SampleRate/OutputFormat pair that `SynthesizeSpeech` accepts, `StartSpeechSynthesisTask` must
reject with `ErrInvalidSampleRate`. Verified failing pre-fix (`_ = forTask` neutering restored via
`cp`, diff-counted before restore; all 3 subtests got `nil` instead of the expected error) and
passing post-fix.

**Gates** (from `services/polly/`, GOTOOLCHAIN=go1.26.6):
```
go build ./services/polly/...            -> ok
go vet ./services/polly/...              -> clean
go test -race -count=1 ./services/polly/... -> ok
golangci-lint run ./services/polly/...   -> 0 issues
go test -race -count=1 ./services/cloudformation/... -> ok (dependent-package check)
```

## 2026-09-08: writeError nil-on-write fall-through audit (gopherstack-246v) -- clean

Part of the sweep following the elasticache fix (gopherstack-8haq): `writeError`
(`handler.go:638`) wraps `c.JSON`, which returns nil on a successful write, so a helper
that rejects via `return writeError(...)` and is called by code storing and checking
its result would get a silent nil back and fall through past the rejection.

**Method (mechanical).** A `go/parser`/`go/ast` fixed-point closure over every non-test
file in this flat package, seeded with `writeError` and `writeBackendError` (the two
local response-writer helpers found by grepping for `func writeError`/`func .*writeError`
in the package). The closure only added one function, `Handler` (`handler.go:123`), whose
`echo.HandlerFunc` closure has a `return writeError(...)` fallback for an unrecognized
route -- final sink set: `{writeError, writeBackendError, Handler}`.

polly's dispatch shape differs from elasticache/cloudfront/mwaa: `Handler()`'s closure
calls `h.dispatch(c, route)`, which returns *raw, unwritten* Go errors from every one of
its 10 handler targets (`synthesizeSpeech`, `startSpeechSynthesisStream`, etc. -- none of
them call `writeError`/`writeBackendError` at all, confirmed by grep: 0 hits outside
`Handler()` and `writeBackendError` itself). `Handler()` then does
`err := h.dispatch(...); if err == nil { return nil }; return h.writeBackendError(c, err)`
-- a store-then-check on `err`, but a safe one: `dispatch` never itself writes a response,
so a non-nil `err` here is always a genuine unwritten error, and the single translation to
a written response (`h.writeBackendError`) happens exactly once, directly returned. This is
already the sentinel-style fix pattern the issue prescribes, just pre-existing.

All 10 production call sites of `{writeError, writeBackendError}` (`handler.go:127,135,
631,635` plus none elsewhere) are `return`-direct. **No instance of the broken shape
exists in polly.** No code changed. Gates:
`GOTOOLCHAIN=go1.27.0 golangci-lint run ./services/polly/...` 0 issues;
`GOTOOLCHAIN=go1.27.0 go test -race ./services/polly/...` ok.
