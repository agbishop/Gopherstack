package iot

import (
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// AssociateTargetsWithJob associates new targets with a continuous job. Real
// AWS IoT returns ResourceNotFoundException for an unknown job ID. Newly
// associated targets are merged into the job's own Targets list (so
// DescribeJob reflects them) and immediately fanned out into QUEUED
// JobExecution rows for any newly-reachable thing, exactly as CreateJob does
// for its initial targets -- real AWS begins rolling out job executions to
// newly associated targets right away for a CONTINUOUS job.
func (b *InMemoryBackend) AssociateTargetsWithJob(
	input *AssociateTargetsWithJobInput,
) (*AssociateTargetsWithJobOutput, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	j, ok := b.jobs.Get(input.JobID)
	if !ok {
		return nil, fmt.Errorf("job %q not found: %w", input.JobID, ErrResourceNotFound)
	}

	b.jobTargets[input.JobID] = append(b.jobTargets[input.JobID], input.Targets...)

	for _, t := range input.Targets {
		j.Targets = appendUnique(j.Targets, t)
	}
	j.LastUpdatedAt = float64(time.Now().Unix())

	b.fanOutJobExecutionsLocked(input.JobID, input.Targets, float64(time.Now().Unix()))

	return &AssociateTargetsWithJobOutput{
		JobID:       input.JobID,
		JobArn:      j.JobARN,
		Description: j.Description,
	}, nil
}

// ListJobExecutionsForJob returns summaries of all executions for a job.
func (b *InMemoryBackend) ListJobExecutionsForJob(jobID string) []*JobExecution {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var out []*JobExecution
	for _, exec := range b.jobExecutions.All() {
		if exec.JobID == jobID {
			cp := *exec
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ThingName < out[j].ThingName })

	return out
}

// ListJobExecutionsForThing returns summaries of all executions for a thing.
func (b *InMemoryBackend) ListJobExecutionsForThing(thingName string) []*JobExecution {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var out []*JobExecution
	for _, exec := range b.jobExecutions.All() {
		if exec.ThingName == thingName {
			cp := *exec
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].JobID < out[j].JobID })

	return out
}

// JobStatus represents the status of a job.
type JobStatus string

const (
	JobStatusInProgress JobStatus = "IN_PROGRESS"
	JobStatusCompleted  JobStatus = "COMPLETED"
	JobStatusFailed     JobStatus = "FAILED"
	JobStatusCanceled   JobStatus = "CANCELED"
	JobStatusDeletion   JobStatus = "DELETION_IN_PROGRESS"
)

// JobExecutionStatus represents the status of a job execution on a thing.
type JobExecutionStatus string

const (
	JobExecQueued     JobExecutionStatus = "QUEUED"
	JobExecInProgress JobExecutionStatus = "IN_PROGRESS"
	JobExecSucceeded  JobExecutionStatus = "SUCCEEDED"
	JobExecFailed     JobExecutionStatus = "FAILED"
	JobExecCanceled   JobExecutionStatus = "CANCELED"
	JobExecRejected   JobExecutionStatus = "REJECTED"
	JobExecRemoved    JobExecutionStatus = "REMOVED"
)

// AbortConfig holds abort criteria for a job.
type AbortConfig struct {
	CriteriaList []AbortCriteria `json:"criteriaList,omitempty"`
}

// AbortCriteria is a single abort criterion.
type AbortCriteria struct {
	Action                    string  `json:"action,omitempty"`
	FailureType               string  `json:"failureType,omitempty"`
	MinNumberOfExecutedThings int     `json:"minNumberOfExecutedThings,omitempty"`
	ThresholdPercentage       float64 `json:"thresholdPercentage,omitempty"`
}

// JobExecutionsRolloutConfig holds rollout config for a job.
type JobExecutionsRolloutConfig struct {
	MaximumPerMinute int `json:"maximumPerMinute,omitempty"`
}

// TimeoutConfig holds timeout settings for a job.
type TimeoutConfig struct {
	InProgressTimeoutInMinutes int64 `json:"inProgressTimeoutInMinutes,omitempty"`
}

// RetryCriteria is a single retry criterion for a job
// (aws-sdk-go-v2/service/iot/types.RetryCriteria).
type RetryCriteria struct {
	FailureType     string `json:"failureType,omitempty"`
	NumberOfRetries int32  `json:"numberOfRetries,omitempty"`
}

// JobExecutionsRetryConfig determines how many retries are allowed for each
// failure type for a job (types.JobExecutionsRetryConfig).
type JobExecutionsRetryConfig struct {
	CriteriaList []RetryCriteria `json:"criteriaList,omitempty"`
}

func cloneJobExecutionsRetryConfig(c *JobExecutionsRetryConfig) *JobExecutionsRetryConfig {
	if c == nil {
		return nil
	}
	cp := *c
	cp.CriteriaList = append([]RetryCriteria(nil), c.CriteriaList...)

	return &cp
}

// PresignedURLConfig holds configuration for pre-signed S3 job-document URLs
// (types.PresignedUrlConfig).
type PresignedURLConfig struct {
	RoleARN      string `json:"roleArn,omitempty"`
	ExpiresInSec int64  `json:"expiresInSec,omitempty"`
}

// MaintenanceWindow is a single recurring maintenance window within a job's
// SchedulingConfig (types.MaintenanceWindow).
type MaintenanceWindow struct {
	StartTime         string `json:"startTime,omitempty"`
	DurationInMinutes int32  `json:"durationInMinutes,omitempty"`
}

// SchedulingConfig schedules a job for a future date/time and configures the
// end behavior for each job execution (types.SchedulingConfig).
type SchedulingConfig struct {
	StartTime          string              `json:"startTime,omitempty"`
	EndTime            string              `json:"endTime,omitempty"`
	EndBehavior        string              `json:"endBehavior,omitempty"`
	MaintenanceWindows []MaintenanceWindow `json:"maintenanceWindows,omitempty"`
}

func cloneSchedulingConfig(c *SchedulingConfig) *SchedulingConfig {
	if c == nil {
		return nil
	}
	cp := *c
	cp.MaintenanceWindows = append([]MaintenanceWindow(nil), c.MaintenanceWindows...)

	return &cp
}

// JobProcessDetails rolls up per-target JobExecution status counts for a job
// (types.JobProcessDetails). Unlike Job's other fields, this is never stored
// on the Job itself -- it is computed on demand from the backend's real
// per-target JobExecution rows (see [InMemoryBackend.jobProcessDetailsLocked])
// so it always reflects current execution state instead of going stale.
type JobProcessDetails struct {
	ProcessingTargets        []string `json:"processingTargets,omitempty"`
	NumberOfQueuedThings     int64    `json:"numberOfQueuedThings"`
	NumberOfInProgressThings int64    `json:"numberOfInProgressThings"`
	NumberOfSucceededThings  int64    `json:"numberOfSucceededThings"`
	NumberOfFailedThings     int64    `json:"numberOfFailedThings"`
	NumberOfRejectedThings   int64    `json:"numberOfRejectedThings"`
	NumberOfCanceledThings   int64    `json:"numberOfCanceledThings"`
	NumberOfRemovedThings    int64    `json:"numberOfRemovedThings"`
	NumberOfTimedOutThings   int64    `json:"numberOfTimedOutThings"`
}

// Job represents an IoT job.
//
// Tags, Document, and DocumentSource are internal-only (json:"-"): real
// AWS IoT's Job shape (types.Job, awsRestjson1_deserializeDocumentJob,
// v1.77.4) has none of these three — tags are a separate ListTagsForResource
// concept, the job document is only returned via GetJobDocument, and
// documentSource is a top-level DescribeJobOutput field. They're kept here
// purely as backend storage and must never leak into a JSON response that
// embeds the whole Job struct.
type Job struct {
	Tags                       map[string]string           `json:"-"`
	DocumentParameters         map[string]string           `json:"documentParameters,omitempty"`
	AbortConfig                *AbortConfig                `json:"abortConfig,omitempty"`
	JobExecutionsRolloutConfig *JobExecutionsRolloutConfig `json:"jobExecutionsRolloutConfig,omitempty"`
	TimeoutConfig              *TimeoutConfig              `json:"timeoutConfig,omitempty"`
	JobExecutionsRetryConfig   *JobExecutionsRetryConfig   `json:"jobExecutionsRetryConfig,omitempty"`
	PresignedURLConfig         *PresignedURLConfig         `json:"presignedUrlConfig,omitempty"`
	SchedulingConfig           *SchedulingConfig           `json:"schedulingConfig,omitempty"`
	// JobProcessDetails is never persisted on the stored Job -- it is
	// computed fresh from the backend's real per-target JobExecution rows
	// each time DescribeJob runs (see jobProcessDetailsLocked) and attached
	// only to the response clone.
	JobProcessDetails          *JobProcessDetails `json:"jobProcessDetails,omitempty"`
	DestinationPackageVersions []string           `json:"destinationPackageVersions,omitempty"`
	JobID                      string             `json:"jobId"`
	JobARN                     string             `json:"jobArn"`
	Description                string             `json:"description,omitempty"`
	Document                   string             `json:"-"`
	DocumentSource             string             `json:"-"`
	JobTemplateARN             string             `json:"jobTemplateArn,omitempty"`
	Status                     JobStatus          `json:"status"`
	TargetSelection            string             `json:"targetSelection,omitempty"`
	Targets                    []string           `json:"targets,omitempty"`
	CreatedAt                  float64            `json:"createdAt,omitempty"`
	LastUpdatedAt              float64            `json:"lastUpdatedAt,omitempty"`
	CompletedAt                float64            `json:"completedAt,omitempty"`
}

// JobExecutionStatusDetails holds free-form status detail name/value pairs
// for a job execution (aws-sdk-go-v2/service/iot/types.
// JobExecutionStatusDetails).
type JobExecutionStatusDetails struct {
	DetailsMap map[string]string `json:"detailsMap,omitempty"`
}

// JobExecution represents a single job execution on a thing.
//
// ThingName is internal-only storage used for lookups (jobExecKey,
// ListJobExecutionsForThing) — real AWS's JobExecution wire shape has only
// "thingArn" (awsRestjson1_deserializeDocumentJobExecution, v1.77.4). Wire
// builders (handler_jobs.go) must compute ThingArn via
// [InMemoryBackend.ThingARN] rather than serializing this struct directly;
// ThingName keeps a normal json tag so Snapshot/Restore still round-trips it.
type JobExecution struct {
	StatusDetails                    *JobExecutionStatusDetails `json:"statusDetails,omitempty"`
	JobID                            string                     `json:"jobId"`
	ThingName                        string                     `json:"thingName"`
	Status                           JobExecutionStatus         `json:"status"`
	ExecutionNumber                  int64                      `json:"executionNumber,omitempty"`
	QueuedAt                         float64                    `json:"queuedAt,omitempty"`
	StartedAt                        float64                    `json:"startedAt,omitempty"`
	LastUpdatedAt                    float64                    `json:"lastUpdatedAt,omitempty"`
	ApproximateSecondsBeforeTimedOut int64                      `json:"approximateSecondsBeforeTimedOut,omitempty"`
	VersionNumber                    int64                      `json:"versionNumber,omitempty"`
	ForceCanceled                    bool                       `json:"forceCanceled,omitempty"`
}

func cloneJob(j *Job) *Job {
	cp := *j
	cp.Targets = append([]string(nil), j.Targets...)
	cp.DestinationPackageVersions = append([]string(nil), j.DestinationPackageVersions...)
	if j.Tags != nil {
		cp.Tags = maps.Clone(j.Tags)
	}
	cp.JobExecutionsRetryConfig = cloneJobExecutionsRetryConfig(j.JobExecutionsRetryConfig)
	cp.SchedulingConfig = cloneSchedulingConfig(j.SchedulingConfig)
	if j.PresignedURLConfig != nil {
		p := *j.PresignedURLConfig
		cp.PresignedURLConfig = &p
	}
	if j.JobProcessDetails != nil {
		d := *j.JobProcessDetails
		d.ProcessingTargets = append([]string(nil), j.JobProcessDetails.ProcessingTargets...)
		cp.JobProcessDetails = &d
	}

	return &cp
}

func (b *InMemoryBackend) jobARN(jobID string) string {
	return arn.Build("iot", b.region, b.accountID, fmt.Sprintf("job/%s", jobID))
}

// ThingARN builds a Thing's ARN from its name, matching the format computed
// elsewhere for real Things (see store.go). Used to derive JobExecution's
// real wire field "thingArn" from the internally-stored ThingName -- this
// does not require the Thing to still exist, matching real AWS's behavior
// of continuing to report the (deterministic) ARN for a job execution even
// after its target Thing has since been deleted.
func (b *InMemoryBackend) ThingARN(thingName string) string {
	return arn.Build("iot", b.region, b.accountID, fmt.Sprintf("thing/%s", thingName))
}

// CreateJobInput holds input for CreateJob.
type CreateJobInput struct {
	// []types.Tag on the wire, not a map (serializers.go:2862, aws-sdk-go-v2/service/iot@v1.77.4).
	Tags                       []tags.KV                   `json:"tags,omitempty"`
	DocumentParameters         map[string]string           `json:"documentParameters,omitempty"`
	AbortConfig                *AbortConfig                `json:"abortConfig,omitempty"`
	JobExecutionsRolloutConfig *JobExecutionsRolloutConfig `json:"jobExecutionsRolloutConfig,omitempty"`
	TimeoutConfig              *TimeoutConfig              `json:"timeoutConfig,omitempty"`
	JobExecutionsRetryConfig   *JobExecutionsRetryConfig   `json:"jobExecutionsRetryConfig,omitempty"`
	PresignedURLConfig         *PresignedURLConfig         `json:"presignedUrlConfig,omitempty"`
	SchedulingConfig           *SchedulingConfig           `json:"schedulingConfig,omitempty"`
	DestinationPackageVersions []string                    `json:"destinationPackageVersions,omitempty"`
	JobID                      string                      `json:"jobId"`
	Description                string                      `json:"description,omitempty"`
	Document                   string                      `json:"document,omitempty"`
	DocumentSource             string                      `json:"documentSource,omitempty"`
	JobTemplateARN             string                      `json:"jobTemplateArn,omitempty"`
	TargetSelection            string                      `json:"targetSelection,omitempty"`
	Targets                    []string                    `json:"targets"`
}

func (b *InMemoryBackend) CreateJob(input *CreateJobInput) (*Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.jobs.Has(input.JobID) {
		return nil, fmt.Errorf("job %q already exists: %w", input.JobID, ErrAlreadyExists)
	}
	now := float64(time.Now().Unix())
	j := &Job{
		JobID:                      input.JobID,
		JobARN:                     b.jobARN(input.JobID),
		Description:                input.Description,
		Document:                   input.Document,
		DocumentSource:             input.DocumentSource,
		JobTemplateARN:             input.JobTemplateARN,
		TargetSelection:            input.TargetSelection,
		Targets:                    append([]string(nil), input.Targets...),
		AbortConfig:                input.AbortConfig,
		JobExecutionsRolloutConfig: input.JobExecutionsRolloutConfig,
		TimeoutConfig:              input.TimeoutConfig,
		JobExecutionsRetryConfig:   cloneJobExecutionsRetryConfig(input.JobExecutionsRetryConfig),
		PresignedURLConfig:         input.PresignedURLConfig,
		SchedulingConfig:           cloneSchedulingConfig(input.SchedulingConfig),
		DestinationPackageVersions: append([]string(nil), input.DestinationPackageVersions...),
		Tags:                       tags.MapFromKV(input.Tags),
		DocumentParameters:         input.DocumentParameters,
		Status:                     JobStatusInProgress,
		CreatedAt:                  now,
		LastUpdatedAt:              now,
	}
	b.jobs.Put(j)
	b.putResourceTagsLocked(j.JobARN, j.Tags)

	// Real AWS IoT creates a QUEUED JobExecution row for every thing the job
	// targets (directly, or as a member of a targeted thing group) at
	// CreateJob time -- this is what makes DescribeJobExecution/
	// ListJobExecutionsForJob/ListJobExecutionsForThing/CancelJobExecution
	// meaningful. See fanOutJobExecutionsLocked.
	b.fanOutJobExecutionsLocked(j.JobID, input.Targets, now)

	return cloneJob(j), nil
}

// parseJobTargetARN splits a job target ARN into its resource type and name
// (e.g. "arn:aws:iot:us-east-1:123:thing/foo" -> ("thing", "foo")). Real AWS
// IoT job Targets are always thing or thing-group ARNs (confirmed against
// CreateJobInput.Targets' doc comment: "A list of things and thing groups to
// which the job should be sent"). Returns ("", "") for anything that doesn't
// parse as a "<type>/<name>" resource.
func parseJobTargetARN(target string) (string, string) {
	resource := target
	if idx := strings.LastIndex(target, ":"); idx != -1 {
		resource = target[idx+1:]
	}

	parts := strings.SplitN(resource, "/", twoparts)
	if len(parts) != twoparts {
		return "", ""
	}

	return parts[0], parts[1]
}

// resolveJobTargetThingNamesLocked expands a job's target ARNs (thing or
// thing-group ARNs) into the concrete, deduplicated set of thing names that
// should get a JobExecution. A thing-group target expands to that group's
// direct members only -- matching ListThingsInThingGroup's own non-recursive
// membership semantics, since this backend does not track recursive/dynamic
// group membership. Unparseable targets, and thing-group targets with no
// members, contribute no thing names. Must be called with b.mu held.
func (b *InMemoryBackend) resolveJobTargetThingNamesLocked(targets []string) []string {
	seen := make(map[string]bool)
	var out []string

	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}

	for _, target := range targets {
		resourceType, name := parseJobTargetARN(target)
		switch resourceType {
		case "thing":
			add(name)
		case "thinggroup":
			for _, member := range b.thingGroupMembers[name] {
				add(member)
			}
		}
	}
	sort.Strings(out)

	return out
}

// fanOutJobExecutionsLocked creates a QUEUED JobExecution for every thing
// name resolved from targets that doesn't already have one for jobID
// (idempotent -- safe to call again with a superset of previously-resolved
// targets, e.g. from AssociateTargetsWithJob). Must be called with b.mu held
// (write).
func (b *InMemoryBackend) fanOutJobExecutionsLocked(jobID string, targets []string, now float64) {
	for _, thingName := range b.resolveJobTargetThingNamesLocked(targets) {
		key := jobExecKey(jobID, thingName)
		if b.jobExecutions.Has(key) {
			continue
		}

		b.jobExecutions.Put(&JobExecution{
			JobID:           jobID,
			ThingName:       thingName,
			Status:          JobExecQueued,
			ExecutionNumber: 1,
			VersionNumber:   1,
			QueuedAt:        now,
			LastUpdatedAt:   now,
		})
	}
}

// jobProcessDetailsLocked computes real per-target JobExecution status
// rollup counts for jobID (types.JobProcessDetails), scanning the backend's
// actual JobExecution rows rather than tracking a separately-maintained
// counter (which would risk drifting out of sync). Must be called with b.mu
// held (read or write).
func (b *InMemoryBackend) jobProcessDetailsLocked(jobID string) *JobProcessDetails {
	details := &JobProcessDetails{}

	var processing []string

	for _, exec := range b.jobExecutions.All() {
		if exec.JobID != jobID {
			continue
		}

		switch exec.Status {
		case JobExecQueued:
			details.NumberOfQueuedThings++
			processing = append(processing, exec.ThingName)
		case JobExecInProgress:
			details.NumberOfInProgressThings++
			processing = append(processing, exec.ThingName)
		case JobExecSucceeded:
			details.NumberOfSucceededThings++
		case JobExecFailed:
			details.NumberOfFailedThings++
		case JobExecRejected:
			details.NumberOfRejectedThings++
		case JobExecCanceled:
			details.NumberOfCanceledThings++
		case JobExecRemoved:
			details.NumberOfRemovedThings++
		}
	}
	sort.Strings(processing)
	details.ProcessingTargets = processing

	return details
}

func (b *InMemoryBackend) DescribeJob(jobID string) (*Job, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	j, ok := b.jobs.Get(jobID)
	if !ok {
		return nil, fmt.Errorf("job %q not found: %w", jobID, ErrResourceNotFound)
	}

	out := cloneJob(j)
	out.JobProcessDetails = b.jobProcessDetailsLocked(jobID)

	return out, nil
}

func (b *InMemoryBackend) ListJobs() []*Job {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]*Job, 0, b.jobs.Len())
	for _, v := range b.jobs.Snapshot() {
		out = append(out, cloneJob(v))
	}

	return out
}

// UpdateJobInput is the input for UpdateJob. Mirrors types (v1.77.4)
// beyond description: AbortConfig/JobExecutionsRolloutConfig/TimeoutConfig/
// JobExecutionsRetryConfig/PresignedURLConfig, all previously silently
// dropped -- UpdateJob only ever applied Description.
type UpdateJobInput struct {
	AbortConfig                *AbortConfig
	JobExecutionsRolloutConfig *JobExecutionsRolloutConfig
	TimeoutConfig              *TimeoutConfig
	JobExecutionsRetryConfig   *JobExecutionsRetryConfig
	PresignedURLConfig         *PresignedURLConfig
	Description                string
}

func (b *InMemoryBackend) UpdateJob(jobID string, input *UpdateJobInput) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	j, ok := b.jobs.Get(jobID)
	if !ok {
		return fmt.Errorf("job %q not found: %w", jobID, ErrResourceNotFound)
	}
	if input.Description != "" {
		j.Description = input.Description
	}
	if input.AbortConfig != nil {
		j.AbortConfig = input.AbortConfig
	}
	if input.JobExecutionsRolloutConfig != nil {
		j.JobExecutionsRolloutConfig = input.JobExecutionsRolloutConfig
	}
	if input.TimeoutConfig != nil {
		j.TimeoutConfig = input.TimeoutConfig
	}
	if input.JobExecutionsRetryConfig != nil {
		j.JobExecutionsRetryConfig = cloneJobExecutionsRetryConfig(input.JobExecutionsRetryConfig)
	}
	if input.PresignedURLConfig != nil {
		j.PresignedURLConfig = input.PresignedURLConfig
	}
	j.LastUpdatedAt = float64(time.Now().Unix())

	return nil
}

// CancelJob cancels a job. Real AWS IoT rejects canceling a job already in a
// terminal state (CancelJobInput has no Force-independent override for this
// -- Force only affects whether IN_PROGRESS job EXECUTIONS are canceled,
// confirmed against CancelJobInput's docs, v1.77.4); this previously set
// Status unconditionally, silently "re-canceling" an already-COMPLETED or
// already-CANCELED job instead of returning InvalidStateTransitionException,
// the same class of terminal-state guard CancelJobExecution/CancelAuditTask
// already enforce.
func (b *InMemoryBackend) CancelJob(jobID, _ string) (*Job, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	j, ok := b.jobs.Get(jobID)
	if !ok {
		return nil, fmt.Errorf("job %q not found: %w", jobID, ErrResourceNotFound)
	}
	if j.Status != JobStatusInProgress {
		return nil, fmt.Errorf("%w: job %q is already in state %s", ErrInvalidStateTransition, jobID, j.Status)
	}
	j.Status = JobStatusCanceled
	j.LastUpdatedAt = float64(time.Now().Unix())

	return cloneJob(j), nil
}

func (b *InMemoryBackend) DeleteJob(jobID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.jobs.Has(jobID) {
		return fmt.Errorf("job %q not found: %w", jobID, ErrResourceNotFound)
	}
	b.jobs.Delete(jobID)
	delete(b.resourceTags, b.jobARN(jobID))
	// Remove executions. Preserves the original key-prefix check (including
	// its edge case: a JobExecution with an empty ThingName is NOT deleted,
	// since its key jobID+"|" has length exactly len(jobID)+1) byte-for-byte.
	for _, exec := range b.jobExecutions.All() {
		k := jobExecKey(exec.JobID, exec.ThingName)
		if len(k) > len(jobID)+1 && k[:len(jobID)] == jobID {
			b.jobExecutions.Delete(k)
		}
	}

	return nil
}

func (b *InMemoryBackend) GetJobDocument(jobID string) (string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	j, ok := b.jobs.Get(jobID)
	if !ok {
		return "", fmt.Errorf("job %q not found: %w", jobID, ErrResourceNotFound)
	}

	return j.Document, nil
}

func jobExecKey(jobID, thingName string) string {
	return jobID + "|" + thingName
}

// AddJobExecutionInternal seeds a job execution directly into the backend
// for testing (mirrors AddAuditTaskInternal/AddServerlessCacheInternal):
// lets tests exercise states (e.g. IN_PROGRESS) that CancelJobExecution's
// own create-on-miss fallback can never produce, since it always creates in
// CANCELED state.
func (b *InMemoryBackend) AddJobExecutionInternal(e *JobExecution) {
	b.mu.Lock()
	defer b.mu.Unlock()

	cp := *e
	b.jobExecutions.Put(&cp)
}

func (b *InMemoryBackend) DescribeJobExecution(jobID, thingName string) (*JobExecution, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	key := jobExecKey(jobID, thingName)
	exec, ok := b.jobExecutions.Get(key)
	if !ok {
		return nil, fmt.Errorf(
			"job execution for job %q / thing %q not found: %w",
			jobID,
			thingName,
			ErrResourceNotFound,
		)
	}
	cp := *exec

	return &cp, nil
}

// CancelJobExecutionOptions carries CancelJobExecution's optional fields,
// matching real CancelJobExecutionInput's force/statusDetails/
// expectedVersion members.
type CancelJobExecutionOptions struct {
	StatusDetails   map[string]string
	Force           bool
	ExpectedVersion int64 // 0 means "not specified"
}

// CancelJobExecution cancels a job execution. Real AWS IoT rejects
// canceling an IN_PROGRESS execution unless force=true
// (InvalidStateTransitionException), and rejects a mismatched
// expectedVersion (VersionConflictException) —
// awsRestjson1_deserializeOpErrorCancelJobExecution recognizes exactly
// those plus InvalidRequestException/ResourceNotFoundException.
//
// CreateJob/AssociateTargetsWithJob fan a QUEUED JobExecution out to every
// resolved target (fanOutJobExecutionsLocked), so the (jobID, thingName)
// pair normally already exists. The exception is a thing-group target with
// no members at CreateJob time (a thing joining later would lazily start an
// execution on real AWS, which this emulator doesn't simulate on
// AddThingToThingGroup) — there, an execution is created directly in
// CANCELED state as a defensive fallback.
func (b *InMemoryBackend) CancelJobExecution(jobID, thingName string, opts CancelJobExecutionOptions) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := float64(time.Now().Unix())
	key := jobExecKey(jobID, thingName)

	exec, ok := b.jobExecutions.Get(key)
	if !ok {
		b.jobExecutions.Put(&JobExecution{
			JobID:           jobID,
			ThingName:       thingName,
			Status:          JobExecCanceled,
			ExecutionNumber: 1,
			VersionNumber:   1,
			ForceCanceled:   opts.Force,
			StatusDetails:   statusDetailsFromMap(opts.StatusDetails),
			QueuedAt:        now,
			LastUpdatedAt:   now,
		})

		return nil
	}

	if opts.ExpectedVersion != 0 && opts.ExpectedVersion != exec.VersionNumber {
		return fmt.Errorf(
			"%w: job execution for job %q / thing %q is at version %d, expected %d",
			ErrVersionConflict, jobID, thingName, exec.VersionNumber, opts.ExpectedVersion,
		)
	}

	if exec.Status == JobExecInProgress && !opts.Force {
		return fmt.Errorf(
			"%w: job execution for job %q / thing %q is IN_PROGRESS, set force=true to cancel it",
			ErrInvalidStateTransition, jobID, thingName,
		)
	}

	exec.Status = JobExecCanceled
	exec.ForceCanceled = opts.Force
	exec.LastUpdatedAt = now
	exec.VersionNumber++

	if len(opts.StatusDetails) > 0 {
		if exec.StatusDetails == nil {
			exec.StatusDetails = &JobExecutionStatusDetails{DetailsMap: map[string]string{}}
		}

		maps.Copy(exec.StatusDetails.DetailsMap, opts.StatusDetails)
	}

	return nil
}

// statusDetailsFromMap wraps a possibly-nil map into a
// *JobExecutionStatusDetails, or nil when the map is empty (matching real
// AWS's "statusDetails absent when never set" behavior).
func statusDetailsFromMap(m map[string]string) *JobExecutionStatusDetails {
	if len(m) == 0 {
		return nil
	}

	cp := make(map[string]string, len(m))
	maps.Copy(cp, m)

	return &JobExecutionStatusDetails{DetailsMap: cp}
}

// isTerminalJobExecutionStatus reports whether status is one of the
// statuses real AWS IoT considers terminal for DeleteJobExecution's
// force-required guard (SUCCEEDED, FAILED, REJECTED, REMOVED, CANCELED --
// everything except QUEUED/IN_PROGRESS, per api_op_DeleteJobExecution.go's
// doc comment).
func isTerminalJobExecutionStatus(status JobExecutionStatus) bool {
	switch status {
	case JobExecSucceeded, JobExecFailed, JobExecRejected, JobExecRemoved, JobExecCanceled:
		return true
	case JobExecQueued, JobExecInProgress:
		return false
	default:
		return false
	}
}

// DeleteJobExecution deletes a job execution. Real AWS IoT rejects deleting
// a non-terminal (QUEUED/IN_PROGRESS) execution unless force=true
// (InvalidStateTransitionException); deleting an execution that does not
// exist is idempotent (matches real AWS, which also returns success for an
// already-absent execution rather than ResourceNotFoundException, since
// deletion is the natural end state).
func (b *InMemoryBackend) DeleteJobExecution(jobID, thingName string, force bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := jobExecKey(jobID, thingName)

	exec, ok := b.jobExecutions.Get(key)
	if !ok {
		return nil
	}

	if !force && !isTerminalJobExecutionStatus(exec.Status) {
		return fmt.Errorf(
			"%w: job execution for job %q / thing %q is %s, set force=true to delete it",
			ErrInvalidStateTransition, jobID, thingName, exec.Status,
		)
	}

	b.jobExecutions.Delete(key)

	return nil
}

// JobTemplate represents an IoT job template.
//
// Tags is internal-only (json:"-"): DescribeJobTemplateOutput
// (awsRestjson1_deserializeOpDocumentDescribeJobTemplateOutput, v1.77.4)
// has no "tags" field — tags are a separate ListTagsForResource concept.
//
// MaintenanceWindows is a TOP-LEVEL field here, unlike Job's
// SchedulingConfig.MaintenanceWindows nesting: JobTemplate has no
// SchedulingConfig wrapper at all (neither DescribeJobTemplateOutput nor
// CreateJobTemplateInput has a schedulingConfig field).
type JobTemplate struct {
	Tags                       map[string]string           `json:"-"`
	AbortConfig                *AbortConfig                `json:"abortConfig,omitempty"`
	JobExecutionsRolloutConfig *JobExecutionsRolloutConfig `json:"jobExecutionsRolloutConfig,omitempty"`
	TimeoutConfig              *TimeoutConfig              `json:"timeoutConfig,omitempty"`
	JobExecutionsRetryConfig   *JobExecutionsRetryConfig   `json:"jobExecutionsRetryConfig,omitempty"`
	PresignedURLConfig         *PresignedURLConfig         `json:"presignedUrlConfig,omitempty"`
	JobTemplateARN             string                      `json:"jobTemplateArn"`
	JobTemplateID              string                      `json:"jobTemplateId"`
	Description                string                      `json:"description,omitempty"`
	Document                   string                      `json:"document,omitempty"`
	DocumentSource             string                      `json:"documentSource,omitempty"`
	MaintenanceWindows         []MaintenanceWindow         `json:"maintenanceWindows,omitempty"`
	DestinationPackageVersions []string                    `json:"destinationPackageVersions,omitempty"`
	CreatedAt                  float64                     `json:"createdAt,omitempty"`
}

func cloneJobTemplate(jt *JobTemplate) *JobTemplate {
	cp := *jt
	cp.JobExecutionsRetryConfig = cloneJobExecutionsRetryConfig(jt.JobExecutionsRetryConfig)
	cp.DestinationPackageVersions = append([]string(nil), jt.DestinationPackageVersions...)
	cp.MaintenanceWindows = append([]MaintenanceWindow(nil), jt.MaintenanceWindows...)
	if jt.PresignedURLConfig != nil {
		p := *jt.PresignedURLConfig
		cp.PresignedURLConfig = &p
	}

	return &cp
}

func (b *InMemoryBackend) jobTemplateARN(id string) string {
	return arn.Build("iot", b.region, b.accountID, fmt.Sprintf("jobtemplate/%s", id))
}

// CreateJobTemplateInput holds input for CreateJobTemplate.
type CreateJobTemplateInput struct {
	// []types.Tag on the wire, not a map (serializers.go:3051, aws-sdk-go-v2/service/iot@v1.77.4).
	Tags                       []tags.KV                   `json:"tags,omitempty"`
	AbortConfig                *AbortConfig                `json:"abortConfig,omitempty"`
	JobExecutionsRolloutConfig *JobExecutionsRolloutConfig `json:"jobExecutionsRolloutConfig,omitempty"`
	TimeoutConfig              *TimeoutConfig              `json:"timeoutConfig,omitempty"`
	JobExecutionsRetryConfig   *JobExecutionsRetryConfig   `json:"jobExecutionsRetryConfig,omitempty"`
	PresignedURLConfig         *PresignedURLConfig         `json:"presignedUrlConfig,omitempty"`
	Description                string                      `json:"description,omitempty"`
	JobTemplateID              string                      `json:"jobTemplateId"`
	Document                   string                      `json:"document,omitempty"`
	DocumentSource             string                      `json:"documentSource,omitempty"`
	JobARN                     string                      `json:"jobArn,omitempty"`
	MaintenanceWindows         []MaintenanceWindow         `json:"maintenanceWindows,omitempty"`
	DestinationPackageVersions []string                    `json:"destinationPackageVersions,omitempty"`
}

func (b *InMemoryBackend) CreateJobTemplate(input *CreateJobTemplateInput) (*JobTemplate, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.jobTemplates.Has(input.JobTemplateID) {
		return nil, fmt.Errorf(
			"job template %q already exists: %w",
			input.JobTemplateID,
			ErrAlreadyExists,
		)
	}
	jt := &JobTemplate{
		JobTemplateID:              input.JobTemplateID,
		JobTemplateARN:             b.jobTemplateARN(input.JobTemplateID),
		Description:                input.Description,
		Document:                   input.Document,
		DocumentSource:             input.DocumentSource,
		AbortConfig:                input.AbortConfig,
		JobExecutionsRolloutConfig: input.JobExecutionsRolloutConfig,
		TimeoutConfig:              input.TimeoutConfig,
		JobExecutionsRetryConfig:   cloneJobExecutionsRetryConfig(input.JobExecutionsRetryConfig),
		PresignedURLConfig:         input.PresignedURLConfig,
		DestinationPackageVersions: append([]string(nil), input.DestinationPackageVersions...),
		MaintenanceWindows:         append([]MaintenanceWindow(nil), input.MaintenanceWindows...),
		Tags:                       tags.MapFromKV(input.Tags),
		CreatedAt:                  float64(time.Now().Unix()),
	}
	b.jobTemplates.Put(jt)
	b.putResourceTagsLocked(jt.JobTemplateARN, jt.Tags)

	return cloneJobTemplate(jt), nil
}

func (b *InMemoryBackend) DescribeJobTemplate(id string) (*JobTemplate, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	jt, ok := b.jobTemplates.Get(id)
	if !ok {
		return nil, fmt.Errorf("job template %q not found: %w", id, ErrResourceNotFound)
	}

	return cloneJobTemplate(jt), nil
}

func (b *InMemoryBackend) ListJobTemplates() []*JobTemplate {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]*JobTemplate, 0, b.jobTemplates.Len())
	for _, v := range b.jobTemplates.Snapshot() {
		out = append(out, cloneJobTemplate(v))
	}

	return out
}

func (b *InMemoryBackend) DeleteJobTemplate(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.jobTemplates.Has(id) {
		return fmt.Errorf("job template %q not found: %w", id, ErrResourceNotFound)
	}
	b.jobTemplates.Delete(id)
	delete(b.resourceTags, b.jobTemplateARN(id))

	return nil
}
