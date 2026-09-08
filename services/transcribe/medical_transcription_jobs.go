package transcribe

import (
	"fmt"
	"slices"
	"sort"
	"time"
)

// supportedMedicalSpecialties returns the set of supported medical specialties.
func supportedMedicalSpecialties() []string { return []string{"PRIMARYCARE"} }

// supportedMedicalTypes returns the set of supported medical transcription types.
func supportedMedicalTypes() []string { return []string{"CONVERSATION", "DICTATION"} }

// supportedMedicalContentIDTypes returns the set of medical content identification types.
func supportedMedicalContentIDTypes() []string { return []string{"PHI"} }

// validateMedicalSpecialty checks that a medical specialty is valid.
func validateMedicalSpecialty(specialty string) error {
	if specialty == "" {
		return fmt.Errorf("%w: Specialty is required for medical transcription", ErrValidation)
	}

	if !slices.Contains(supportedMedicalSpecialties(), specialty) {
		return fmt.Errorf("%w: Specialty %q must be one of %v",
			ErrValidation, specialty, supportedMedicalSpecialties())
	}

	return nil
}

// validateMedicalType checks that a medical transcription type is valid.
func validateMedicalType(typ string) error {
	if typ == "" {
		return fmt.Errorf("%w: Type is required for medical transcription", ErrValidation)
	}

	if !slices.Contains(supportedMedicalTypes(), typ) {
		return fmt.Errorf("%w: Type %q must be one of %v",
			ErrValidation, typ, supportedMedicalTypes())
	}

	return nil
}

// StartMedicalTranscriptionJob creates a new Medical Transcription job.
func (b *InMemoryBackend) StartMedicalTranscriptionJob(
	input *MedicalTranscriptionJob,
) (*MedicalTranscriptionJob, error) {
	if err := validateJobName(input.MedicalTranscriptionJobName); err != nil {
		return nil, fmt.Errorf("%w: MedicalTranscriptionJobName is required", ErrValidation)
	}

	if input.LanguageCode == "" {
		return nil, fmt.Errorf("%w: LanguageCode is required", ErrValidation)
	}

	if input.LanguageCode != "en-US" {
		return nil, fmt.Errorf("%w: LanguageCode must be en-US for medical transcription jobs", ErrValidation)
	}

	if err := validateMedicalSpecialty(input.Specialty); err != nil {
		return nil, err
	}

	if err := validateMedicalType(input.Type); err != nil {
		return nil, err
	}

	if err := validateMediaFormat(input.MediaFormat); err != nil {
		return nil, err
	}

	if err := validateMediaSampleRateHertz(input.MediaSampleRateHertz); err != nil {
		return nil, err
	}

	if input.MedicalContentIdentificationType != "" {
		if !slices.Contains(supportedMedicalContentIDTypes(), input.MedicalContentIdentificationType) {
			return nil, fmt.Errorf("%w: MedicalContentIdentificationType %q must be PHI",
				ErrValidation, input.MedicalContentIdentificationType)
		}
	}

	b.mu.Lock("StartMedicalTranscriptionJob")
	defer b.mu.Unlock()

	if b.medicalTranscriptionJobs.Has(input.MedicalTranscriptionJobName) {
		return nil, fmt.Errorf(
			"%w: medical transcription job %s already exists",
			ErrAlreadyExists,
			input.MedicalTranscriptionJobName,
		)
	}

	now := time.Now()
	job := *input
	job.TranscriptionJobStatus = jobStatusCompleted
	job.CreationTime = now
	job.StartTime = now
	job.CompletionTime = now
	job.TranscriptJSON = synthesizeTranscriptJSON(input.MedicalTranscriptionJobName,
		"Medical transcription result for "+input.MedicalTranscriptionJobName+".")
	b.medicalTranscriptionJobs.Put(&job)
	b.recordResourceTagsLocked(
		resourceARN(resourceTypeMedicalTranscriptionJob, job.MedicalTranscriptionJobName), job.Tags,
	)

	cp := job

	return &cp, nil
}

// GetMedicalTranscriptionJob returns a Medical Transcription job by name.
func (b *InMemoryBackend) GetMedicalTranscriptionJob(
	jobName string,
) (*MedicalTranscriptionJob, error) {
	b.mu.RLock("GetMedicalTranscriptionJob")
	defer b.mu.RUnlock()

	job, ok := b.medicalTranscriptionJobs.Get(jobName)
	if !ok {
		return nil, fmt.Errorf("%w: medical transcription job %s not found", ErrNotFound, jobName)
	}

	cp := *job
	cp.Tags = b.liveTagsLocked(resourceARN(resourceTypeMedicalTranscriptionJob, jobName))

	return &cp, nil
}

// ListMedicalTranscriptionJobs returns Medical Transcription jobs with optional status
// filter, name substring filter, and pagination.
func (b *InMemoryBackend) ListMedicalTranscriptionJobs(
	statusFilter, nameContains, nextToken string, maxResults int32,
) ([]MedicalTranscriptionJob, string) {
	b.mu.RLock("ListMedicalTranscriptionJobs")
	defer b.mu.RUnlock()

	all := make([]MedicalTranscriptionJob, 0, b.medicalTranscriptionJobs.Len())
	for _, j := range b.medicalTranscriptionJobs.All() {
		if (statusFilter == "" || j.TranscriptionJobStatus == statusFilter) &&
			matchesNameContains(j.MedicalTranscriptionJobName, nameContains) {
			all = append(all, *j)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].MedicalTranscriptionJobName < all[j].MedicalTranscriptionJobName
	})

	return paginateList(all, nextToken, maxResults)
}

// DeleteMedicalTranscriptionJob removes a medical transcription job by name.
func (b *InMemoryBackend) DeleteMedicalTranscriptionJob(jobName string) error {
	if jobName == "" {
		return fmt.Errorf("%w: MedicalTranscriptionJobName is required", ErrValidation)
	}

	b.mu.Lock("DeleteMedicalTranscriptionJob")
	defer b.mu.Unlock()

	if !b.medicalTranscriptionJobs.Delete(jobName) {
		return fmt.Errorf("%w: medical transcription job %s not found", ErrNotFound, jobName)
	}

	b.forgetResourceTagsLocked(resourceARN(resourceTypeMedicalTranscriptionJob, jobName))

	return nil
}

// AddMedicalTranscriptionJobInternal seeds a Medical Transcription job directly (test helper).
func (b *InMemoryBackend) AddMedicalTranscriptionJobInternal(job *MedicalTranscriptionJob) {
	b.mu.Lock("AddMedicalTranscriptionJobInternal")
	defer b.mu.Unlock()

	cp := *job
	b.medicalTranscriptionJobs.Put(&cp)
}
