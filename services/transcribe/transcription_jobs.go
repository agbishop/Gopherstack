package transcribe

import (
	"fmt"
	"slices"
	"sort"
	"time"
)

// maxSpeakerLabelsMin is the minimum number of speakers for diarization.
const maxSpeakerLabelsMin = 2

// maxSpeakerLabelsMax is the maximum number of speakers for diarization.
const maxSpeakerLabelsMax = 10

// maxAlternativesMin is the minimum number of alternative transcripts.
const maxAlternativesMin = 2

// maxAlternativesMax is the maximum number of alternative transcripts.
const maxAlternativesMax = 10

// syntheticIdentifiedLanguageScore is the confidence score returned when language identification is enabled.
const syntheticIdentifiedLanguageScore = float32(0.97)

// supportedVocabularyFilterMethods returns the set of vocabulary filter methods.
func supportedVocabularyFilterMethods() []string {
	return []string{"remove", "mask", "tag"}
}

// supportedContentRedactionTypes returns the set of content redaction types.
func supportedContentRedactionTypes() []string { return []string{"PII"} }

// supportedRedactionOutputs returns the set of redaction output options.
func supportedRedactionOutputs() []string { return []string{"redacted", "redacted_and_unredacted"} }

// supportedSubtitleFormats returns the set of subtitle output formats.
func supportedSubtitleFormats() []string { return []string{"vtt", "srt"} }

// StartTranscriptionJob creates a new transcription job with synthetic results.
// The input struct carries all supported fields; validation is performed before storage.
func (b *InMemoryBackend) StartTranscriptionJob(input *TranscriptionJob) (*TranscriptionJob, error) {
	if err := validateTranscriptionJobInput(input); err != nil {
		return nil, err
	}

	b.mu.Lock("StartTranscriptionJob")
	defer b.mu.Unlock()

	if b.jobs.Has(input.JobName) {
		return nil, fmt.Errorf("%w: job %s already exists", ErrAlreadyExists, input.JobName)
	}

	job := buildCompletedTranscriptionJob(input)
	if input.JobExecutionSettings != nil && input.JobExecutionSettings.AllowDeferredExecution {
		job = buildQueuedTranscriptionJob(input)
	}

	b.jobs.Put(&job)
	b.recordResourceTagsLocked(resourceARN(resourceTypeTranscriptionJob, job.JobName), job.Tags)
	cp := job

	return &cp, nil
}

// buildQueuedTranscriptionJob initialises a deferred job before execution starts.
func buildQueuedTranscriptionJob(input *TranscriptionJob) TranscriptionJob {
	job := *input
	job.JobStatus = jobStatusQueued
	job.CreationTime = time.Now()

	return job
}

// buildCompletedTranscriptionJob initialises a job as COMPLETED with synthetic results.
func buildCompletedTranscriptionJob(input *TranscriptionJob) TranscriptionJob {
	now := time.Now()
	mediaURI := ""
	if input.Media.MediaFileURI != "" {
		mediaURI = input.Media.MediaFileURI
	}
	transcriptText := deriveTranscriptText(input.JobName, mediaURI)

	job := *input
	job.JobStatus = jobStatusCompleted
	job.CreationTime = now
	job.StartTime = now
	job.CompletionTime = now
	job.TranscriptText = transcriptText
	job.TranscriptJSON = synthesizeTranscriptJSON(input.JobName, transcriptText)

	// Synthesize subtitle file URIs when subtitles were requested.
	if input.Subtitles != nil && len(input.Subtitles.Formats) > 0 {
		uris := make([]string, 0, len(input.Subtitles.Formats))
		for _, f := range input.Subtitles.Formats {
			uris = append(uris, "s3://synthetic-transcripts/"+input.JobName+"."+f)
		}

		job.Subtitles = &SubtitlesOutput{
			Formats:          input.Subtitles.Formats,
			SubtitleFileURIs: uris,
			OutputStartIndex: input.Subtitles.OutputStartIndex,
		}
	}

	// Synthesize identified language score when language identification is enabled.
	if input.IdentifyLanguage || input.IdentifyMultipleLanguages {
		job.LanguageCode = resolveEffectiveLanguageCode(input.LanguageCode, input.LanguageOptions)
		job.IdentifiedLanguageScore = syntheticIdentifiedLanguageScore
	}

	return job
}

func advanceDeferredTranscriptionJob(job *TranscriptionJob) {
	switch job.JobStatus {
	case jobStatusQueued:
		job.JobStatus = jobStatusInProgress
		job.StartTime = time.Now()
	case jobStatusInProgress:
		job.CompletionTime = time.Now()
		if job.FailureReason != "" {
			job.JobStatus = jobStatusFailed

			return
		}

		completed := buildCompletedTranscriptionJob(job)
		completed.CreationTime = job.CreationTime
		completed.StartTime = job.StartTime
		completed.CompletionTime = time.Now()
		*job = completed
	}
}

// resolveEffectiveLanguageCode returns the best language code to use when language identification is on.
func resolveEffectiveLanguageCode(languageCode string, options []string) string {
	if languageCode != "" {
		return languageCode
	}

	if len(options) > 0 {
		return options[0]
	}

	return "en-US"
}

// subtitlesInputFromOutput converts a SubtitlesOutput back to input for validation.
func subtitlesInputFromOutput(s *SubtitlesOutput) *SubtitlesInput {
	if s == nil {
		return nil
	}

	return &SubtitlesInput{Formats: s.Formats, OutputStartIndex: s.OutputStartIndex}
}

// GetTranscriptionJob returns a transcription job by name.
func (b *InMemoryBackend) GetTranscriptionJob(jobName string) (*TranscriptionJob, error) {
	b.mu.Lock("GetTranscriptionJob")
	defer b.mu.Unlock()

	job, ok := b.jobs.Get(jobName)
	if !ok {
		return nil, fmt.Errorf("%w: job %s not found", ErrNotFound, jobName)
	}

	advanceDeferredTranscriptionJob(job)
	cp := *job
	cp.Tags = b.liveTagsLocked(resourceARN(resourceTypeTranscriptionJob, job.JobName))

	return &cp, nil
}

// ListTranscriptionJobs returns transcription jobs, optionally filtered by status and
// name substring, with pagination.
func (b *InMemoryBackend) ListTranscriptionJobs(
	statusFilter, nameContains, nextToken string, maxResults int32,
) ([]TranscriptionJob, string) {
	b.mu.RLock("ListTranscriptionJobs")
	defer b.mu.RUnlock()

	all := make([]TranscriptionJob, 0, b.jobs.Len())
	for _, j := range b.jobs.All() {
		if (statusFilter == "" || j.JobStatus == statusFilter) && matchesNameContains(j.JobName, nameContains) {
			all = append(all, *j)
		}
	}

	sort.Slice(all, func(i, j int) bool { return all[i].JobName < all[j].JobName })

	return paginateList(all, nextToken, maxResults)
}

// DeleteTranscriptionJob removes a transcription job by name.
func (b *InMemoryBackend) DeleteTranscriptionJob(jobName string) error {
	if jobName == "" {
		return fmt.Errorf("%w: TranscriptionJobName is required", ErrValidation)
	}

	b.mu.Lock("DeleteTranscriptionJob")
	defer b.mu.Unlock()

	if !b.jobs.Delete(jobName) {
		return fmt.Errorf("%w: job %s not found", ErrNotFound, jobName)
	}

	b.forgetResourceTagsLocked(resourceARN(resourceTypeTranscriptionJob, jobName))

	return nil
}

func validateTranscriptionJobInput(input *TranscriptionJob) error {
	if err := validateJobName(input.JobName); err != nil {
		return err
	}

	if !input.IdentifyLanguage && !input.IdentifyMultipleLanguages && input.LanguageCode == "" {
		return fmt.Errorf(
			"%w: LanguageCode is required (or set IdentifyLanguage/IdentifyMultipleLanguages)",
			ErrValidation,
		)
	}

	if err := validateLanguageCode(input.LanguageCode); err != nil {
		return err
	}

	if err := validateMediaFormat(input.MediaFormat); err != nil {
		return err
	}

	if err := validateMediaSampleRateHertz(input.MediaSampleRateHertz); err != nil {
		return err
	}

	if err := validateSettings(input.Settings); err != nil {
		return err
	}

	if err := validateContentRedaction(input.ContentRedaction); err != nil {
		return err
	}

	if err := validateJobExecutionSettings(input.JobExecutionSettings); err != nil {
		return err
	}

	if err := validateSubtitles(subtitlesInputFromOutput(input.Subtitles)); err != nil {
		return err
	}

	if err := validateLanguageOptions(input.LanguageOptions); err != nil {
		return err
	}

	if err := validateLanguageIDSettings(input.LanguageIDSettings, input.IdentifyMultipleLanguages); err != nil {
		return err
	}

	return nil
}

// validateSpeakerLabels enforces the bidirectional requirement documented on
// Settings: ShowSpeakerLabels and MaxSpeakerLabels must be set together.
func validateSpeakerLabels(s *TranscriptionSettings) error {
	if s.MaxSpeakerLabels != 0 && !s.ShowSpeakerLabels {
		return fmt.Errorf("%w: ShowSpeakerLabels must be true when MaxSpeakerLabels is set", ErrValidation)
	}

	if s.ShowSpeakerLabels {
		if s.MaxSpeakerLabels == 0 {
			return fmt.Errorf("%w: MaxSpeakerLabels is required when ShowSpeakerLabels is true", ErrValidation)
		}

		if s.MaxSpeakerLabels < maxSpeakerLabelsMin || s.MaxSpeakerLabels > maxSpeakerLabelsMax {
			return fmt.Errorf("%w: MaxSpeakerLabels must be between %d and %d",
				ErrValidation, maxSpeakerLabelsMin, maxSpeakerLabelsMax)
		}
	}

	return nil
}

// validateAlternatives enforces the bidirectional requirement documented on
// Settings: ShowAlternatives and MaxAlternatives must be set together.
func validateAlternatives(s *TranscriptionSettings) error {
	if s.MaxAlternatives != 0 && !s.ShowAlternatives {
		return fmt.Errorf("%w: ShowAlternatives must be true when MaxAlternatives is set", ErrValidation)
	}

	if s.ShowAlternatives {
		if s.MaxAlternatives == 0 {
			return fmt.Errorf("%w: MaxAlternatives is required when ShowAlternatives is true", ErrValidation)
		}

		if s.MaxAlternatives < maxAlternativesMin || s.MaxAlternatives > maxAlternativesMax {
			return fmt.Errorf("%w: MaxAlternatives must be between %d and %d",
				ErrValidation, maxAlternativesMin, maxAlternativesMax)
		}
	}

	return nil
}

// validateSettings validates Settings fields when Settings is non-nil.
func validateSettings(s *TranscriptionSettings) error {
	if s == nil {
		return nil
	}

	if err := validateSpeakerLabels(s); err != nil {
		return err
	}

	if err := validateAlternatives(s); err != nil {
		return err
	}

	if s.VocabularyFilterMethod != "" &&
		!slices.Contains(supportedVocabularyFilterMethods(), s.VocabularyFilterMethod) {
		return fmt.Errorf("%w: VocabularyFilterMethod %q must be one of %v",
			ErrValidation, s.VocabularyFilterMethod, supportedVocabularyFilterMethods())
	}

	return nil
}

// validateContentRedaction validates ContentRedaction fields when non-nil.
func validateContentRedaction(cr *ContentRedaction) error {
	if cr == nil {
		return nil
	}

	if cr.RedactionType == "" {
		return fmt.Errorf("%w: ContentRedaction.RedactionType is required", ErrValidation)
	}

	if !slices.Contains(supportedContentRedactionTypes(), cr.RedactionType) {
		return fmt.Errorf("%w: ContentRedaction.RedactionType %q must be one of %v",
			ErrValidation, cr.RedactionType, supportedContentRedactionTypes())
	}

	if cr.RedactionOutput == "" {
		return fmt.Errorf("%w: ContentRedaction.RedactionOutput is required", ErrValidation)
	}

	if !slices.Contains(supportedRedactionOutputs(), cr.RedactionOutput) {
		return fmt.Errorf("%w: ContentRedaction.RedactionOutput %q must be one of %v",
			ErrValidation, cr.RedactionOutput, supportedRedactionOutputs())
	}

	return nil
}

// validateJobExecutionSettings enforces the role requirement for deferred jobs.
func validateJobExecutionSettings(settings *JobExecutionSettings) error {
	if settings == nil || !settings.AllowDeferredExecution {
		return nil
	}

	if settings.DataAccessRoleArn == "" {
		return fmt.Errorf(
			"%w: JobExecutionSettings.DataAccessRoleArn is required when AllowDeferredExecution is true",
			ErrValidation,
		)
	}

	return nil
}

// validateSubtitles validates Subtitles input fields when non-nil.
func validateSubtitles(s *SubtitlesInput) error {
	if s == nil {
		return nil
	}

	for _, f := range s.Formats {
		if !slices.Contains(supportedSubtitleFormats(), f) {
			return fmt.Errorf("%w: Subtitles.Formats contains unsupported format %q; must be vtt or srt",
				ErrValidation, f)
		}
	}

	return nil
}
