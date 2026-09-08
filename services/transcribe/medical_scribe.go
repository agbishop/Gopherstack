package transcribe

import (
	"fmt"
	"slices"
	"sort"
	"time"
)

// supportedMedicalScribeParticipantRoles returns the set of participant roles
// accepted for a Medical Scribe channel definition.
func supportedMedicalScribeParticipantRoles() []string { return []string{"PATIENT", "CLINICIAN"} }

// validateMedicalScribeSettings enforces the constraints documented on
// StartMedicalScribeJob: Settings is required and must set exactly one of
// ShowSpeakerLabels or ChannelIdentification to true; ShowSpeakerLabels=true
// requires MaxSpeakerLabels to also be set; ChannelDefinitions must be present
// if and only if ChannelIdentification is true.
func validateMedicalScribeSettings(
	settings *MedicalScribeSettings, channelDefs []MedicalScribeChannelDefinition,
) error {
	if settings == nil {
		return fmt.Errorf("%w: Settings is required for MedicalScribeJob", ErrValidation)
	}

	if settings.ShowSpeakerLabels == settings.ChannelIdentification {
		return fmt.Errorf(
			"%w: exactly one of Settings.ShowSpeakerLabels or Settings.ChannelIdentification must be true",
			ErrValidation,
		)
	}

	if settings.ShowSpeakerLabels && settings.MaxSpeakerLabels == 0 {
		return fmt.Errorf("%w: Settings.MaxSpeakerLabels is required when ShowSpeakerLabels is true", ErrValidation)
	}

	if (len(channelDefs) > 0) != settings.ChannelIdentification {
		return fmt.Errorf(
			"%w: ChannelDefinitions must be set if and only if Settings.ChannelIdentification is true",
			ErrValidation,
		)
	}

	for i := range channelDefs {
		if channelDefs[i].ParticipantRole == "" {
			return fmt.Errorf("%w: each ChannelDefinition must have a ParticipantRole", ErrValidation)
		}

		if !slices.Contains(supportedMedicalScribeParticipantRoles(), channelDefs[i].ParticipantRole) {
			return fmt.Errorf("%w: ChannelDefinition.ParticipantRole %q must be one of %v",
				ErrValidation, channelDefs[i].ParticipantRole, supportedMedicalScribeParticipantRoles())
		}
	}

	return nil
}

// StartMedicalScribeJob creates a new Medical Scribe job.
func (b *InMemoryBackend) StartMedicalScribeJob(input *MedicalScribeJob) (*MedicalScribeJob, error) {
	if err := validateJobName(input.MedicalScribeJobName); err != nil {
		return nil, fmt.Errorf("%w: MedicalScribeJobName is required", ErrValidation)
	}

	if input.DataAccessRoleArn == "" {
		return nil, fmt.Errorf("%w: DataAccessRoleArn is required for MedicalScribeJob", ErrValidation)
	}

	if input.OutputBucketName == "" {
		return nil, fmt.Errorf("%w: OutputBucketName is required for MedicalScribeJob", ErrValidation)
	}

	if err := validateMedicalScribeSettings(input.Settings, input.ChannelDefinitions); err != nil {
		return nil, err
	}

	b.mu.Lock("StartMedicalScribeJob")
	defer b.mu.Unlock()

	if b.medicalScribeJobs.Has(input.MedicalScribeJobName) {
		return nil, fmt.Errorf(
			"%w: medical scribe job %s already exists",
			ErrAlreadyExists,
			input.MedicalScribeJobName,
		)
	}

	now := time.Now()
	job := *input
	job.MedicalScribeJobStatus = jobStatusCompleted
	job.CreationTime = now
	job.StartTime = now
	job.CompletionTime = now
	b.medicalScribeJobs.Put(&job)
	b.recordResourceTagsLocked(resourceARN(resourceTypeMedicalScribeJob, job.MedicalScribeJobName), job.Tags)

	cp := job

	return &cp, nil
}

// GetMedicalScribeJob returns a Medical Scribe job by name.
func (b *InMemoryBackend) GetMedicalScribeJob(jobName string) (*MedicalScribeJob, error) {
	b.mu.RLock("GetMedicalScribeJob")
	defer b.mu.RUnlock()

	job, ok := b.medicalScribeJobs.Get(jobName)
	if !ok {
		return nil, fmt.Errorf("%w: medical scribe job %s not found", ErrNotFound, jobName)
	}

	cp := *job
	cp.Tags = b.liveTagsLocked(resourceARN(resourceTypeMedicalScribeJob, jobName))

	return &cp, nil
}

// ListMedicalScribeJobs returns Medical Scribe jobs with optional status filter, name
// substring filter, and pagination.
func (b *InMemoryBackend) ListMedicalScribeJobs(
	statusFilter, nameContains, nextToken string, maxResults int32,
) ([]MedicalScribeJob, string) {
	b.mu.RLock("ListMedicalScribeJobs")
	defer b.mu.RUnlock()

	all := make([]MedicalScribeJob, 0, b.medicalScribeJobs.Len())
	for _, j := range b.medicalScribeJobs.All() {
		if (statusFilter == "" || j.MedicalScribeJobStatus == statusFilter) &&
			matchesNameContains(j.MedicalScribeJobName, nameContains) {
			all = append(all, *j)
		}
	}

	sort.Slice(
		all,
		func(i, j int) bool { return all[i].MedicalScribeJobName < all[j].MedicalScribeJobName },
	)

	return paginateList(all, nextToken, maxResults)
}

// DeleteMedicalScribeJob removes a Medical Scribe job by name.
func (b *InMemoryBackend) DeleteMedicalScribeJob(jobName string) error {
	if jobName == "" {
		return fmt.Errorf("%w: MedicalScribeJobName is required", ErrValidation)
	}

	b.mu.Lock("DeleteMedicalScribeJob")
	defer b.mu.Unlock()

	if !b.medicalScribeJobs.Delete(jobName) {
		return fmt.Errorf("%w: medical scribe job %s not found", ErrNotFound, jobName)
	}

	b.forgetResourceTagsLocked(resourceARN(resourceTypeMedicalScribeJob, jobName))

	return nil
}

// AddMedicalScribeJobInternal seeds a Medical Scribe job directly (test helper).
func (b *InMemoryBackend) AddMedicalScribeJobInternal(job *MedicalScribeJob) {
	b.mu.Lock("AddMedicalScribeJobInternal")
	defer b.mu.Unlock()

	cp := *job
	b.medicalScribeJobs.Put(&cp)
}
