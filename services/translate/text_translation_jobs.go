package translate

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// StartTextTranslationJob creates a new async translation job.
func (b *InMemoryBackend) StartTextTranslationJob(
	jobName, dataAccessRoleARN, sourceLang string,
	targetLangs, terminologyNames, parallelDataNames []string,
	inputCfg, outputCfg, settings map[string]any,
	tags map[string]string,
) (*TranslationJob, error) {
	b.mu.Lock("StartTextTranslationJob")
	defer b.mu.Unlock()

	// A referenced TerminologyNames/ParallelDataNames entry that doesn't
	// exist is the only named-resource lookup StartTextTranslationJob
	// performs, and the operation models ResourceNotFoundException
	// (api-2.json) for exactly this: "For a list of available ... resources,
	// use List{Terminologies,ParallelData}" in the real API reference.
	for _, name := range terminologyNames {
		if !b.terminologies.Has(name) {
			return nil, fmt.Errorf("%w: terminology %q not found", ErrNotFound, name)
		}
	}

	for _, name := range parallelDataNames {
		if !b.parallelData.Has(name) {
			return nil, fmt.Errorf("%w: parallel data %q not found", ErrNotFound, name)
		}
	}

	jobID := uuid.New().String()

	job := &TranslationJob{
		JobID:             jobID,
		JobName:           jobName,
		JobStatus:         jobStatusSubmitted,
		DataAccessRoleARN: dataAccessRoleARN,
		SourceLanguage:    sourceLang,
		TargetLanguages:   targetLangs,
		TerminologyNames:  terminologyNames,
		ParallelDataNames: parallelDataNames,
		InputDataConfig:   inputCfg,
		OutputDataConfig:  outputCfg,
		Settings:          settings,
		Tags:              tags,
		SubmittedAt:       time.Now().UTC(),
		shouldFail:        strings.Contains(strings.ToLower(jobName), failedJobNameMarker),
	}
	b.jobs.Put(job)

	return job, nil
}

// StopTextTranslationJob requests stop of a translation job.
func (b *InMemoryBackend) StopTextTranslationJob(jobID string) (*TranslationJob, error) {
	b.mu.Lock("StopTextTranslationJob")
	defer b.mu.Unlock()

	job, ok := b.jobs.Get(jobID)
	if !ok {
		return nil, fmt.Errorf("%w: job %q not found", ErrNotFound, jobID)
	}

	// StopTextTranslationJob models no InvalidRequestException or
	// InvalidParameterValueException at all -- only ResourceNotFoundException,
	// TooManyRequestsException, and InternalServerException
	// (deserializers.go's awsAwsjson11_deserializeOpErrorStopTextTranslationJob)
	// -- so a job that isn't stoppable is not a client error: Stop is
	// idempotent and just reports the job's current state back unchanged.
	if job.JobStatus == jobStatusInProgress || job.JobStatus == jobStatusSubmitted {
		job.stopRequested = true
		job.JobStatus = jobStatusStopRequested
		job.EndAt = time.Now().UTC()
	}

	return job, nil
}

// DescribeTextTranslationJob retrieves a translation job and advances it one
// step through its lifecycle, matching services/comprehend's DescribeJob
// pattern: real batch translation jobs move from SUBMITTED to IN_PROGRESS to
// a terminal state (COMPLETED/FAILED, or STOPPED once stop was requested)
// asynchronously. Without this, a job started via StartTextTranslationJob
// would sit at its initial status forever and SDK callers polling
// DescribeTextTranslationJob (the documented way to track job progress) would
// never observe completion.
func (b *InMemoryBackend) DescribeTextTranslationJob(jobID string) (*TranslationJob, error) {
	b.mu.Lock("DescribeTextTranslationJob")
	defer b.mu.Unlock()

	job, ok := b.jobs.Get(jobID)
	if !ok {
		return nil, fmt.Errorf("%w: job %q not found", ErrNotFound, jobID)
	}

	advanceJob(job)

	return job, nil
}

// advanceJob moves job one step through its lifecycle. Called from
// DescribeTextTranslationJob so that each poll makes progress, the same
// convention services/comprehend uses for its analysis jobs.
func advanceJob(job *TranslationJob) {
	switch job.JobStatus {
	case jobStatusSubmitted:
		job.JobStatus = jobStatusInProgress
	case jobStatusInProgress:
		if job.shouldFail {
			job.JobStatus = jobStatusFailed
			job.Message = "simulated translation failure"
		} else {
			job.JobStatus = jobStatusCompleted
		}

		job.EndAt = time.Now().UTC()
	case jobStatusStopRequested:
		job.JobStatus = jobStatusStopped
		job.EndAt = time.Now().UTC()
	}
}

// TextTranslationJobFilter mirrors types.TextTranslationJobFilter
// (api_op_ListTextTranslationJobs.go): JobName/JobStatus/SubmittedAfterTime/
// SubmittedBeforeTime, each optional and independently zero-valued when unset.
type TextTranslationJobFilter struct {
	SubmittedAfterTime  *time.Time
	SubmittedBeforeTime *time.Time
	JobName             string
	JobStatus           string
}

// matchesJobFilter reports whether job satisfies every constraint set on
// filter (each field is independently optional).
func matchesJobFilter(job *TranslationJob, filter TextTranslationJobFilter) bool {
	if filter.JobName != "" && job.JobName != filter.JobName {
		return false
	}

	if filter.JobStatus != "" && job.JobStatus != filter.JobStatus {
		return false
	}

	if filter.SubmittedAfterTime != nil && !job.SubmittedAt.After(*filter.SubmittedAfterTime) {
		return false
	}

	if filter.SubmittedBeforeTime != nil && !job.SubmittedAt.Before(*filter.SubmittedBeforeTime) {
		return false
	}

	return true
}

// sortJobs orders jobs by SubmittedAt following the two documented cases on
// TextTranslationJobFilter's own fields: SubmittedBeforeTime returns
// ascending (oldest to newest); SubmittedAfterTime returns descending
// (newest to oldest). No case is documented for an unfiltered/JobName/
// JobStatus-only request, so this backend defaults to the same descending
// order SubmittedAfterTime uses, for a consistent "most recent first"
// default across every other filter combination. JobID is a stable tiebreak
// for equal timestamps.
func sortJobs(jobs []*TranslationJob, ascending bool) {
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].SubmittedAt.Equal(jobs[j].SubmittedAt) {
			return jobs[i].JobID < jobs[j].JobID
		}

		if ascending {
			return jobs[i].SubmittedAt.Before(jobs[j].SubmittedAt)
		}

		return jobs[i].SubmittedAt.After(jobs[j].SubmittedAt)
	})
}

// ListTextTranslationJobs returns a paginated list of translation jobs.
func (b *InMemoryBackend) ListTextTranslationJobs(
	filter TextTranslationJobFilter,
	maxResults int,
	nextToken string,
) ([]*TranslationJob, string) {
	b.mu.RLock("ListTextTranslationJobs")
	defer b.mu.RUnlock()

	jobs := make([]*TranslationJob, 0, b.jobs.Len())

	for _, job := range b.jobs.All() {
		if matchesJobFilter(job, filter) {
			jobs = append(jobs, job)
		}
	}

	sortJobs(jobs, filter.SubmittedBeforeTime != nil)

	ids := make([]string, len(jobs))
	for i, job := range jobs {
		ids[i] = job.JobID
	}

	return paginate(ids, func(id string) *TranslationJob { return tableGet(b.jobs, id) }, maxResults, nextToken)
}
