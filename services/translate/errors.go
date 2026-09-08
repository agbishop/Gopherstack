package translate

import "errors"

// Sentinel errors map 1:1 to Translate's modeled exception shapes
// (aws-sdk-go-v2/service/translate/types/errors.go, confirmed against the
// smithy model's per-operation error lists in
// aws-sdk-go@v1.55.5/models/apis/translate/2017-07-01/api-2.json). handleError
// in handler.go matches each with errors.Is and emits the exact "__type" wire
// code; every exception below is smithy.FaultClient (HTTP 400) per the real
// SDK's ErrorFault() implementations and the AWS API reference's documented
// "HTTP Status Code: 400" for each -- only InternalServerException and
// ServiceUnavailableException are FaultServer (500), and neither has a
// deterministic trigger in this synchronous, single-lock, unbounded
// in-memory emulator (no internal faults, no real service outages) any more
// than TooManyRequestsException (no rate limiting) or
// DetectedLanguageLowConfidenceException (no real Comprehend-backed
// detection). Generic throttling/5xx injection for any operation is
// available instead through the chaos fault-injection system
// (ChaosOperations/ChaosServiceName), matching services/comprehend's
// documented precedent for the same class of unmodeled-but-real exceptions.
// ConcurrentModificationException is the one exception in that "no
// deterministic trigger" family that turned out to have one: this emulator's
// own CREATING/UPDATING -> ACTIVE async lifecycle (parallel_data.go) is a
// real, observable non-terminal window, not a genuine goroutine race -- see
// ErrConcurrentModification below.
var (
	// ErrNotFound is returned when a requested resource is absent. Also used
	// (per the real per-operation error models) for operations whose only
	// modeled client-error exception IS ResourceNotFoundException -- i.e.
	// GetParallelData/DeleteParallelData/StopTextTranslationJob/
	// DescribeTextTranslationJob have no InvalidRequestException or
	// InvalidParameterValueException in their error list at all, so an
	// empty/missing key on those operations must still surface as
	// ResourceNotFoundException, not a validation exception the real
	// operation never throws.
	ErrNotFound = errors.New("ResourceNotFoundException")
	// ErrConflict is returned when a named resource already exists (name
	// conflict on CreateParallelData). Real Amazon Translate has no
	// "ResourceInUseException" shape at all (confirmed: absent from
	// types/errors.go and the smithy model) -- CreateParallelData models
	// ConflictException for exactly this case.
	ErrConflict = errors.New("ConflictException")
	// ErrValidation is returned for invalid/missing request values on
	// operations whose modeled error list includes InvalidRequestException:
	// CreateParallelData, UpdateParallelData, StartTextTranslationJob,
	// ListTextTranslationJobs, TranslateText, TranslateDocument.
	ErrValidation = errors.New("InvalidRequestException")
	// ErrInvalidParameter is returned for invalid/missing request values on
	// operations whose modeled error list includes
	// InvalidParameterValueException but NOT InvalidRequestException:
	// ImportTerminology, GetTerminology, DeleteTerminology,
	// ListTerminologies, GetParallelData, ListParallelData, TagResource,
	// UntagResource, ListTagsForResource, ListLanguages.
	ErrInvalidParameter = errors.New("InvalidParameterValueException")
	// ErrLimitExceeded is returned when a request would exceed a modeled
	// service quota: the 10 MB custom terminology file size limit
	// (ImportTerminology) or the 100,000-byte TranslateDocument document
	// size limit (TranslateDocument), both confirmed against the
	// TerminologyFile/DocumentContent blob shapes' "max" constraints in the
	// smithy model.
	ErrLimitExceeded = errors.New("LimitExceededException")
	// ErrTextSizeLimitExceeded is returned when TranslateText's Text exceeds
	// the 10,000-byte synchronous translation quota (confirmed against
	// BoundedLengthString's "max": 10000 in the smithy model and the Amazon
	// Translate guidelines/quotas page).
	ErrTextSizeLimitExceeded = errors.New("TextSizeLimitExceededException")
	// ErrTooManyTags is returned when a resource would carry more than the
	// 50-tag-per-resource limit (both existing and newly requested tags
	// count), matching TooManyTagsException.
	ErrTooManyTags = errors.New("TooManyTagsException")
	// ErrUnsupportedLanguagePair is returned when TranslateText,
	// TranslateDocument, or StartTextTranslationJob is given a
	// SourceLanguageCode (other than "auto") or target language code not in
	// Translate's supported language list (see handler_languages.go's
	// knownLanguages, the same list ListLanguages serves).
	ErrUnsupportedLanguagePair = errors.New("UnsupportedLanguagePairException")
	// ErrUnsupportedDisplayLanguage is returned when ListLanguages is given
	// a DisplayLanguageCode outside the fixed 10-value enum Translate
	// actually supports for UI localization (de/en/es/fr/it/ja/ko/pt/zh/
	// zh-TW -- confirmed against the DisplayLanguageCode shape's "enum" in
	// the smithy model; this is a much smaller set than the ~75
	// translation-target language codes).
	ErrUnsupportedDisplayLanguage = errors.New("UnsupportedDisplayLanguageCodeException")
	// ErrInvalidFilter is returned when ListTextTranslationJobs is given a
	// Filter.JobStatus value outside the modeled JobStatus enum.
	ErrInvalidFilter = errors.New("InvalidFilterException")
	// ErrConcurrentModification is returned when UpdateParallelData targets a
	// parallel data resource that is still CREATING or UPDATING from a prior
	// call. types/errors.go's ConcurrentModificationException doc: "Another
	// modification is being made. That modification must complete before you
	// can make your change." UpdateParallelData models this exception
	// (deserializers.go's awsAwsjson11_deserializeOpErrorUpdateParallelData).
	ErrConcurrentModification = errors.New("ConcurrentModificationException")
)
