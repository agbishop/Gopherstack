package batch

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	jobStatusSubmitted = "SUBMITTED"
	jobStatusPending   = "PENDING"
	jobStatusRunnable  = "RUNNABLE"
	jobStatusStarting  = "STARTING"
	jobStatusRunning   = "RUNNING"
	jobStatusSucceeded = "SUCCEEDED"
	jobStatusFailed    = "FAILED"

	maxJobNameLength = 128

	// KeyValuesPair filter names shared by ListJobs/ListServiceJobs/
	// ListConsumableResources (api_op_ListJobs.go, api_op_ListServiceJobs.go).
	filterJobName         = "JOB_NAME"
	filterJobDefinition   = "JOB_DEFINITION"
	filterShareIdentifier = "SHARE_IDENTIFIER"
	filterQuotaShareName  = "QUOTA_SHARE_NAME"
	filterBeforeCreatedAt = "BEFORE_CREATED_AT"
	filterAfterCreatedAt  = "AFTER_CREATED_AT"
)

// newConsumableResourceProperties wraps a non-empty requirement list in the
// AWS-shaped ConsumableResourceProperties envelope, returning nil for an
// empty list so the field is omitted from the wire response.
func newConsumableResourceProperties(list []ConsumableResourceProperty) *ConsumableResourceProperties {
	if len(list) == 0 {
		return nil
	}

	listCopy := make([]ConsumableResourceProperty, len(list))
	copy(listCopy, list)

	return &ConsumableResourceProperties{ConsumableResourceList: listCopy}
}

// lookupJobByIDOrARN returns a job by ID or ARN within region using the jobsByARN
// index for O(1) ARN lookup. Caller must hold at least a read lock.
func (b *InMemoryBackend) lookupJobByIDOrARN(region, idOrARN string) (*Job, bool) {
	if j, ok := b.jobs.Get(regionKey(region, idOrARN)); ok {
		return j, true
	}

	if matches := b.jobsByARN.Get(regionKey(region, idOrARN)); len(matches) > 0 {
		return matches[0], true
	}

	return nil, false
}

// parseJobDefRevision splits a short job definition reference into its name
// and (if present) numeric revision, e.g. "my-jd:3" -> ("my-jd", 3, true) and
// "my-jd" -> ("my-jd", 0, false). A non-numeric suffix after the colon is
// treated as part of the name (hasRevision is false) since job definition
// names never contain a colon themselves.
func parseJobDefRevision(ref string) (string, int32, bool) {
	base, revStr, found := strings.Cut(ref, ":")
	if !found {
		return ref, 0, false
	}

	rev, err := strconv.ParseInt(revStr, 10, 32)
	if err != nil {
		return ref, 0, false
	}

	return base, int32(rev), true
}

// lookupJobDefinitionForSubmit resolves the jobDefinition parameter accepted
// by SubmitJob: a full ARN, "name:revision", or a bare name. A bare name
// resolves to the newest ACTIVE revision, matching AWS Batch's documented
// behavior ("If the revision is not specified, then the latest active
// revision is used."). An explicit revision must match exactly and must be
// ACTIVE. Caller must hold at least a read lock.
func (b *InMemoryBackend) lookupJobDefinitionForSubmit(region, ref string) (*JobDefinition, bool) {
	if jd, ok := b.jobDefinitions.Get(regionKey(region, ref)); ok {
		return jd, true
	}

	name, revision, hasRevision := parseJobDefRevision(ref)

	var latest *JobDefinition

	for _, jd := range b.jobDefinitionsByRegion.Get(region) {
		if jd.JobDefinitionName != name || jd.Status != jobDefStatusActive {
			continue
		}

		if hasRevision {
			if jd.Revision == revision {
				return jd, true
			}

			continue
		}

		if latest == nil || jd.Revision > latest.Revision {
			latest = jd
		}
	}

	if hasRevision {
		return nil, false
	}

	return latest, latest != nil
}

// submitJobCopies holds deep copies of the optional, pointer/slice-typed
// SubmitJob inputs so the backend never retains caller-owned memory.
type submitJobCopies struct {
	retryStrategy      *RetryStrategy
	timeout            *JobTimeout
	arrayProperties    *ArrayProperties
	containerOverrides *ContainerOverrides
	dependsOn          []JobDependency
}

// cloneRetryStrategy deep-copies a RetryStrategy, including its
// EvaluateOnExit slice, returning nil for nil input. Shared by SubmitJob and
// RegisterJobDefinition, which both accept a RetryStrategy stored by
// reference.
func cloneRetryStrategy(retryStrategy *RetryStrategy) *RetryStrategy {
	if retryStrategy == nil {
		return nil
	}

	rs := *retryStrategy
	if len(rs.EvaluateOnExit) > 0 {
		exitRules := make([]EvaluateOnExit, len(rs.EvaluateOnExit))
		copy(exitRules, rs.EvaluateOnExit)
		rs.EvaluateOnExit = exitRules
	}

	return &rs
}

// cloneSubmitJobInputs deep-copies the optional SubmitJob parameters that are
// stored by reference (dependsOn, retryStrategy, timeout, arrayProperties,
// containerOverrides).
func cloneSubmitJobInputs(
	dependsOn []JobDependency,
	retryStrategy *RetryStrategy,
	timeout *JobTimeout,
	arrayProperties *ArrayProperties,
	containerOverrides *ContainerOverrides,
) submitJobCopies {
	var out submitJobCopies

	if len(dependsOn) > 0 {
		out.dependsOn = make([]JobDependency, len(dependsOn))
		copy(out.dependsOn, dependsOn)
	}

	out.retryStrategy = cloneRetryStrategy(retryStrategy)

	if timeout != nil {
		tc := *timeout
		out.timeout = &tc
	}

	if arrayProperties != nil {
		ap := *arrayProperties
		out.arrayProperties = &ap
	}

	if containerOverrides != nil {
		co := *containerOverrides
		out.containerOverrides = &co
	}

	return out
}

// SubmitJob submits a new Batch job for execution.
func (b *InMemoryBackend) SubmitJob(
	ctx context.Context,
	name, queue, jobDefinition string,
	tags map[string]string,
	parameters map[string]string,
	dependsOn []JobDependency,
	retryStrategy *RetryStrategy,
	timeout *JobTimeout,
	arrayProperties *ArrayProperties,
	containerOverrides *ContainerOverrides,
	consumableResourceProperties []ConsumableResourceProperty,
	shareIdentifier string,
	schedulingPriorityOverride int32,
	propagateTags bool,
) (*Job, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("SubmitJob")
	defer b.mu.Unlock()

	if len(name) == 0 || len(name) > maxJobNameLength {
		return nil, fmt.Errorf("%w: jobName must be between 1 and %d characters", ErrValidation, maxJobNameLength)
	}

	jq, ok := b.lookupJQByNameOrARN(region, queue)
	if !ok {
		return nil, fmt.Errorf("%w: job queue %s not found", ErrNotFound, queue)
	}

	if jq.State == stateDisabled {
		return nil, fmt.Errorf("%w: job queue %s is %s", ErrValidation, queue, stateDisabled)
	}

	jd, ok := b.lookupJobDefinitionForSubmit(region, jobDefinition)
	if !ok {
		return nil, fmt.Errorf("%w: job definition %s not found", ErrNotFound, jobDefinition)
	}

	if err := validateTags(tags); err != nil {
		return nil, err
	}

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	paramsCopy := maps.Clone(parameters)
	copies := cloneSubmitJobInputs(dependsOn, retryStrategy, timeout, arrayProperties, containerOverrides)

	now := time.Now().UnixMilli()
	jobID := uuid.NewString()
	jobARN := arn.Build("batch", region, b.accountID, "job/"+jobID)

	j := &Job{
		region:  region,
		JobID:   jobID,
		JobARN:  jobARN,
		JobName: name,
		// JobDetail.JobQueue is documented as the ARN of the job queue, not
		// its name; the byQueue index (jobQueueIndexKeyFn) keys off this same
		// field, so callers that look up the index must resolve to the
		// queue's ARN too (see listJobIDsForQueue, DeleteJobQueue,
		// GetJobQueueSnapshot).
		JobQueue: jq.JobQueueArn,
		// AWS resolves the jobDefinition parameter (name, name:revision, or
		// ARN with/without revision) to the definition's ARN; DescribeJobs
		// always returns the ARN here, never the caller's short reference.
		JobDefinition:                jd.JobDefinitionArn,
		Status:                       jobStatusSubmitted,
		CreatedAt:                    now,
		Tags:                         tagsCopy,
		Parameters:                   paramsCopy,
		DependsOn:                    copies.dependsOn,
		RetryStrategy:                copies.retryStrategy,
		Timeout:                      copies.timeout,
		ArrayProperties:              copies.arrayProperties,
		ContainerOverrides:           copies.containerOverrides,
		ConsumableResourceProperties: newConsumableResourceProperties(consumableResourceProperties),
		ShareIdentifier:              shareIdentifier,
		SchedulingPriorityOverride:   schedulingPriorityOverride,
		PropagateTags:                propagateTags,
		// PlatformCapabilities is snapshotted from the resolved job
		// definition at submit time, matching AWS's JobDetail.
		// PlatformCapabilities semantics (defaults to the definition's own
		// setting, not re-derived at describe time).
		PlatformCapabilities: append([]string(nil), jd.PlatformCapabilities...),
	}
	b.jobs.Put(j)

	cp := *j
	cp.Tags = tagsCloneOrEmpty(j.Tags)

	return &cp, nil
}

// listJobIDsForQueue returns job IDs for region, either all jobs sorted by ID
// (queue == "") or jobs scoped to queue sorted by CreatedAt -- matching the
// pre-refactor jobsIdx / jobsByQueue-derived ordering exactly. Caller must
// hold at least a read lock.
func (b *InMemoryBackend) listJobIDsForQueue(region, queue string) ([]string, error) {
	if queue == "" {
		return sortedNames(b.jobsByRegion.Get(region), func(j *Job) string { return j.JobID }), nil
	}

	jq, ok := b.lookupJQByNameOrARN(region, queue)
	if !ok {
		return nil, fmt.Errorf("%w: job queue %s not found", ErrNotFound, queue)
	}

	group := b.jobsByQueueIdx.Get(regionKey(region, jq.JobQueueArn))
	ids := make([]string, len(group))
	for i, j := range group {
		ids[i] = j.JobID
	}

	sort.Slice(ids, func(i, k int) bool {
		ji, _ := b.jobs.Get(regionKey(region, ids[i]))
		jk, _ := b.jobs.Get(regionKey(region, ids[k]))

		return ji.CreatedAt < jk.CreatedAt
	})

	return ids, nil
}

// KeyValueFilter is one KeyValuesPair entry from ListJobsInput.Filters
// (aws-sdk-go-v2/service/batch/types.KeyValuesPair). Name is case sensitive
// per the SDK's own doc comment on KeyValuesPair.
type KeyValueFilter struct {
	Name   string
	Values []string
}

// jobDefinitionNameFromARN extracts the name from a
// "job-definition/<name>:<revision>" ARN resource segment, as built by
// job_definitions.go's RegisterJobDefinition (arn.Build(..., "job-definition/%s:%d", ...)).
func jobDefinitionNameFromARN(jdARN string) string {
	resource := jdARN
	if i := strings.LastIndex(jdARN, "job-definition/"); i >= 0 {
		resource = jdARN[i+len("job-definition/"):]
	}

	if i := strings.LastIndex(resource, ":"); i >= 0 {
		return resource[:i]
	}

	return resource
}

// filterValueMatches reports whether s matches value under ListJobs' shared
// wildcard rule: a trailing '*' is a prefix match, otherwise an exact match.
// caseInsensitive controls whether the comparison folds case (JOB_NAME is
// documented case-insensitive; JOB_DEFINITION and SHARE_IDENTIFIER are not).
func filterValueMatches(s, value string, caseInsensitive bool) bool {
	if caseInsensitive {
		s = strings.ToLower(s)
		value = strings.ToLower(value)
	}

	if prefix, ok := strings.CutSuffix(value, "*"); ok {
		return strings.HasPrefix(s, prefix)
	}

	return s == value
}

// jobMatchesFilterValue reports whether j matches a single value of a
// single-named filter (one of the JOB_NAME/JOB_DEFINITION/SHARE_IDENTIFIER/
// BEFORE_CREATED_AT/AFTER_CREATED_AT filter names documented on
// api_op_ListJobs.go). An unrecognized name matches nothing.
func jobMatchesFilterValue(j *Job, name, v string) bool {
	switch name {
	case filterJobName:
		return filterValueMatches(j.JobName, v, true)
	case filterJobDefinition:
		return jobMatchesJobDefinitionFilter(j, v)
	case filterShareIdentifier:
		return j.ShareIdentifier == v
	case filterBeforeCreatedAt:
		ms, err := strconv.ParseInt(v, 10, 64)

		return err == nil && j.CreatedAt < ms
	case filterAfterCreatedAt:
		ms, err := strconv.ParseInt(v, 10, 64)

		return err == nil && j.CreatedAt > ms
	default:
		return false
	}
}

// jobMatchesJobDefinitionFilter implements the JOB_DEFINITION filter: an ARN
// value is matched exactly (no wildcard support for ARNs, per
// api_op_ListJobs.go: "Asterisk isn't supported when the ARN is used"); a
// bare name matches any revision of that job definition, case sensitively,
// with the same trailing-'*' prefix rule as JOB_NAME.
func jobMatchesJobDefinitionFilter(j *Job, v string) bool {
	if strings.HasPrefix(v, "arn:") {
		return j.JobDefinition == v
	}

	return filterValueMatches(jobDefinitionNameFromARN(j.JobDefinition), v, false)
}

// jobMatchesFilter reports whether j satisfies a single KeyValueFilter entry.
// Values within one entry are OR'd (matches any).
func jobMatchesFilter(j *Job, f KeyValueFilter) bool {
	for _, v := range f.Values {
		if jobMatchesFilterValue(j, f.Name, v) {
			return true
		}
	}

	return false
}

// ListJobs returns job summaries for a queue, optionally filtered by status
// and/or Filters. Matching real AWS Batch's documented ListJobs behavior
// (api_op_ListJobs.go): an unspecified status defaults to RUNNING; when
// Filters is non-empty, status is ignored (jobs of any status are returned)
// unless every filter entry is SHARE_IDENTIFIER, the one documented
// exception where status and Filters combine. Pagination is controlled via
// maxResults and nextToken (token encodes an integer offset).
func (b *InMemoryBackend) ListJobs(
	ctx context.Context,
	queue, status, nextToken string,
	maxResults int32,
	filters []KeyValueFilter,
) ([]*Job, string, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListJobs")
	defer b.mu.RUnlock()

	allKeys, err := b.listJobIDsForQueue(region, queue)
	if err != nil {
		return nil, "", err
	}

	shareIdentifierOnly := len(filters) > 0
	for _, f := range filters {
		if f.Name != filterShareIdentifier {
			shareIdentifierOnly = false

			break
		}
	}

	applyStatus := len(filters) == 0 || shareIdentifierOnly

	wantStatus := status
	if wantStatus == "" {
		wantStatus = jobStatusRunning
	}

	filtered := make([]string, 0, len(allKeys))

	for _, k := range allKeys {
		j, _ := b.jobs.Get(regionKey(region, k))
		if applyStatus && j.Status != wantStatus {
			continue
		}

		matched := true

		for _, f := range filters {
			if !jobMatchesFilter(j, f) {
				matched = false

				break
			}
		}

		if matched {
			filtered = append(filtered, k)
		}
	}

	allKeys = filtered

	pageKeys, next := paginateMapKeys(allKeys, nextToken, maxResults)

	out := make([]*Job, 0, len(pageKeys))
	for _, k := range pageKeys {
		j, _ := b.jobs.Get(regionKey(region, k))
		cp := *j
		cp.Tags = tagsCloneOrEmpty(cp.Tags)
		out = append(out, &cp)
	}

	return out, next, nil
}

// DescribeJobs returns full job details for the given job IDs or ARNs.
func (b *InMemoryBackend) DescribeJobs(ctx context.Context, jobIDs []string) []*Job {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeJobs")
	defer b.mu.RUnlock()

	out := make([]*Job, 0, len(jobIDs))

	for _, id := range jobIDs {
		j, ok := b.lookupJobByIDOrARN(region, id)
		if !ok {
			continue
		}

		cp := *j
		cp.Tags = tagsCloneOrEmpty(j.Tags)
		cp.Container = b.buildJobContainerDetail(region, j)
		out = append(out, &cp)
	}

	return out
}

// buildJobContainerDetail derives the describe-side Container view for a job
// from its resolved job definition's ContainerProperties merged with the
// job's ContainerOverrides, matching aws-sdk-go-v2/service/batch/types.
// JobDetail.Container. Returns nil for multi-node jobs (NodeProperties set)
// or definitions with no ContainerProperties, matching AWS's "for a
// multiple-container job, this object will be empty" behavior. Caller must
// hold at least a read lock.
func (b *InMemoryBackend) buildJobContainerDetail(region string, j *Job) *ContainerDetail {
	jd, ok := b.jobDefinitions.Get(regionKey(region, j.JobDefinition))
	if !ok || jd.ContainerProperties == nil || jd.NodeProperties != nil {
		return nil
	}

	cp := jd.ContainerProperties
	cd := &ContainerDetail{
		LinuxParameters:              cp.LinuxParameters,
		RepositoryCredentials:        cp.RepositoryCredentials,
		RuntimePlatform:              cp.RuntimePlatform,
		EphemeralStorage:             cp.EphemeralStorage,
		FargatePlatformConfiguration: cp.FargatePlatformConfiguration,
		NetworkConfiguration:         cp.NetworkConfiguration,
		LogConfiguration:             cp.LogConfiguration,
		JobRoleArn:                   cp.JobRoleArn,
		ExecutionRoleArn:             cp.ExecutionRoleArn,
		User:                         cp.User,
		InstanceType:                 cp.InstanceType,
		Image:                        cp.Image,
		Command:                      cp.Command,
		Secrets:                      cp.Secrets,
		ResourceRequirements:         cp.ResourceRequirements,
		Ulimits:                      cp.Ulimits,
		MountPoints:                  cp.MountPoints,
		Volumes:                      cp.Volumes,
		Environment:                  cp.Environment,
		Vcpus:                        cp.Vcpus,
		Memory:                       cp.Memory,
		ReadonlyRootFilesystem:       cp.ReadonlyRootFilesystem,
		Privileged:                   cp.Privileged,
	}

	applyContainerOverrides(cd, j.ContainerOverrides)

	// Real AWS assigns a log stream name once the container reaches RUNNING.
	if j.StartedAt != nil {
		cd.LogStreamName = fmt.Sprintf("%s/default/%s", jd.JobDefinitionName, j.JobID)
	}

	return cd
}

// applyContainerOverrides merges SubmitJob's ContainerOverrides onto a
// ContainerDetail derived from a job definition, matching AWS's override
// semantics (instanceType/command/environment/resourceRequirements replace
// the definition's values when set).
func applyContainerOverrides(cd *ContainerDetail, overrides *ContainerOverrides) {
	if overrides == nil {
		return
	}

	if overrides.InstanceType != "" {
		cd.InstanceType = overrides.InstanceType
	}

	if len(overrides.Command) > 0 {
		cd.Command = overrides.Command
	}

	if len(overrides.Environment) > 0 {
		cd.Environment = overrides.Environment
	}

	if len(overrides.ResourceRequirements) > 0 {
		cd.ResourceRequirements = overrides.ResourceRequirements
	}
}

// TerminateJob marks a job as FAILED with the given reason.
// Valid for any non-terminal state. Accepts job ID or ARN.
func (b *InMemoryBackend) TerminateJob(ctx context.Context, idOrARN, reason string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("TerminateJob")
	defer b.mu.Unlock()

	j, ok := b.lookupJobByIDOrARN(region, idOrARN)
	if !ok {
		return fmt.Errorf("%w: job %s not found", ErrNotFound, idOrARN)
	}

	if j.Status == jobStatusSucceeded || j.Status == jobStatusFailed {
		return fmt.Errorf("%w: job %s is already in terminal state %s", ErrValidation, idOrARN, j.Status)
	}

	now := time.Now().UnixMilli()
	j.Status = jobStatusFailed
	j.StatusReason = reason
	j.StoppedAt = &now
	j.IsTerminated = true

	return nil
}

// CancelJob cancels a job in SUBMITTED, PENDING, or RUNNABLE state. A job
// that has already progressed to STARTING or RUNNING is not cancelled, but
// the call still succeeds with no state change -- matching the documented
// AWS behavior (api_op_CancelJob.go: "Jobs that progressed to the STARTING
// or RUNNING state aren't canceled. However, the API operation still
// succeeds, even if no job is canceled. These jobs must be terminated with
// the TerminateJob operation."). Accepts job ID or ARN.
func (b *InMemoryBackend) CancelJob(ctx context.Context, idOrARN, reason string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("CancelJob")
	defer b.mu.Unlock()

	j, ok := b.lookupJobByIDOrARN(region, idOrARN)
	if !ok {
		return fmt.Errorf("%w: job %s not found", ErrNotFound, idOrARN)
	}

	switch j.Status {
	case jobStatusSubmitted, jobStatusPending, jobStatusRunnable:
		now := time.Now().UnixMilli()
		j.Status = jobStatusFailed
		j.StatusReason = reason
		j.StoppedAt = &now
		j.IsCancelled = true

		return nil
	case jobStatusStarting, jobStatusRunning:
		return nil
	default:
		return fmt.Errorf("%w: cannot cancel job %s in %s state", ErrValidation, idOrARN, j.Status)
	}
}

// effectiveTimeoutSeconds returns job's own JobTimeout.AttemptDurationSeconds
// if set, else the job definition's, else 0 (no timeout). Real AWS: SubmitJob's
// own Timeout overrides the job definition's when both are present
// (api_op_SubmitJob.go's Timeout doc: "the timeout configuration for this
// SubmitJob operation... overrides ... the job definition"). Caller must hold
// at least a read lock.
func (b *InMemoryBackend) effectiveTimeoutSeconds(job *Job) int32 {
	if job.Timeout != nil && job.Timeout.AttemptDurationSeconds > 0 {
		return job.Timeout.AttemptDurationSeconds
	}

	jd, ok := b.jobDefinitions.Get(regionKey(job.region, job.JobDefinition))
	if !ok || jd.Timeout == nil {
		return 0
	}

	return jd.Timeout.AttemptDurationSeconds
}

// effectiveRetryAttempts returns job's own RetryStrategy.Attempts if set,
// else the job definition's, else 1 (AWS's default of one attempt -- no
// retry -- when no RetryStrategy is configured). Caller must hold at least a
// read lock.
func (b *InMemoryBackend) effectiveRetryAttempts(job *Job) int32 {
	const defaultAttempts = 1

	if job.RetryStrategy != nil && job.RetryStrategy.Attempts > 0 {
		return job.RetryStrategy.Attempts
	}

	jd, ok := b.jobDefinitions.Get(regionKey(job.region, job.JobDefinition))
	if ok && jd.RetryStrategy != nil && jd.RetryStrategy.Attempts > 0 {
		return jd.RetryStrategy.Attempts
	}

	return defaultAttempts
}

// jobAttemptTimedOutLocked reports whether job's current RUNNING attempt has
// run longer than its effective JobTimeout.AttemptDurationSeconds. Caller
// must hold at least a read lock.
func (b *InMemoryBackend) jobAttemptTimedOutLocked(job *Job) bool {
	if job.StartedAt == nil {
		return false
	}

	timeoutSec := b.effectiveTimeoutSeconds(job)
	if timeoutSec <= 0 {
		return false
	}

	const millisPerSecond = 1000

	elapsedMs := time.Now().UnixMilli() - *job.StartedAt

	return elapsedMs >= int64(timeoutSec)*millisPerSecond
}

// applyAttemptTimeoutLocked resolves a timed-out RUNNING attempt: retries it
// (back to RUNNABLE) if RetryStrategy.Attempts allows another attempt --
// matching the real doc, "If the value of attempts is greater than one, the
// job is retried on failure the same number of attempts as the value" --
// or fails the job for good once attempts are exhausted. Caller must hold
// the write lock.
func (b *InMemoryBackend) applyAttemptTimeoutLocked(job *Job, now int64) {
	const timeoutReason = "job attempt duration exceeded timeout"

	job.Attempts = append(job.Attempts, JobAttempt{
		StartedAt:    job.StartedAt,
		StoppedAt:    &now,
		StatusReason: timeoutReason,
	})

	job.attemptCount++

	if job.attemptCount < b.effectiveRetryAttempts(job) {
		job.Status = jobStatusRunnable
		job.StatusReason = timeoutReason + "; retrying"
		job.StartedAt = nil

		return
	}

	job.Status = jobStatusFailed
	job.StatusReason = timeoutReason
	job.StoppedAt = &now
}
