package macie2

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// CreateClassificationJob creates a new classification job.
func (b *InMemoryBackend) CreateClassificationJob(
	name, description, jobType, clientToken string,
	s3JobDefinition, scheduleFrequency map[string]any,
	allowListIDs, customDataIdentifierIDs, managedDataIdentifierIDs []string,
	managedDataIdentifierSelector string,
	tags map[string]string,
	samplingPercentage int32,
	initialRun bool,
) (string, string, error) {
	b.mu.Lock("CreateClassificationJob")
	defer b.mu.Unlock()

	id := uuid.New().String()
	jobArn := arn.Build("macie2", b.region, b.accountID, fmt.Sprintf("classification-job/%s", id))
	now := time.Now().UTC()

	status := jobStatusRunning
	if jobType == "SCHEDULED" {
		status = jobStatusIdle
	}

	pct := samplingPercentage
	if pct == 0 {
		pct = 100
	}

	b.classificationJobs.Put(&ClassificationJob{
		JobID:                         id,
		Arn:                           jobArn,
		Name:                          name,
		Description:                   description,
		JobType:                       jobType,
		JobStatus:                     status,
		CreatedAt:                     now,
		S3JobDefinition:               s3JobDefinition,
		ScheduleFrequency:             scheduleFrequency,
		AllowListIDs:                  allowListIDs,
		CustomDataIdentifierIDs:       customDataIdentifierIDs,
		ManagedDataIdentifierIDs:      managedDataIdentifierIDs,
		ManagedDataIdentifierSelector: managedDataIdentifierSelector,
		Tags:                          maps.Clone(tags),
		SamplingPercentage:            pct,
		InitialRun:                    initialRun,
		ClientToken:                   clientToken,
		LastRunTime:                   &now,
		LastRunErrorStatus:            &JobLastRunErrorStatus{Code: "NONE"},
		Statistics:                    &JobStatistics{NumberOfRuns: 1},
	})

	if len(tags) > 0 {
		b.tags[jobArn] = maps.Clone(tags)
	}

	return id, jobArn, nil
}

// DescribeClassificationJob returns a classification job by ID.
func (b *InMemoryBackend) DescribeClassificationJob(jobID string) (*ClassificationJob, error) {
	b.mu.RLock("DescribeClassificationJob")
	defer b.mu.RUnlock()

	job, ok := b.classificationJobs.Get(jobID)
	if !ok {
		return nil, ErrClassificationJobNotFound
	}

	cp := *job
	cp.Tags = maps.Clone(job.Tags)

	return &cp, nil
}

// ListJobsSortCriteria mirrors types.ListJobsSortCriteria. AttributeName's
// four defined enum values (createdAt, jobStatus, name, jobType --
// types/enums.go) are all backed by ClassificationJobSummary fields.
type ListJobsSortCriteria struct {
	AttributeName string
	OrderBy       string
}

func sortJobSummaries(result []*ClassificationJobSummary, sortBy *ListJobsSortCriteria) {
	if sortBy == nil {
		sort.Slice(result, func(i, j int) bool {
			if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
				return result[i].CreatedAt.Before(result[j].CreatedAt)
			}

			return result[i].JobID < result[j].JobID
		})

		return
	}

	desc := sortBy.OrderBy == sortOrderDesc

	sort.Slice(result, func(i, j int) bool {
		var less, tied bool

		switch sortBy.AttributeName {
		case keyCreatedAt:
			less = result[i].CreatedAt.Before(result[j].CreatedAt)
			tied = result[i].CreatedAt.Equal(result[j].CreatedAt)
		case keyJobStatus:
			less, tied = result[i].JobStatus < result[j].JobStatus, result[i].JobStatus == result[j].JobStatus
		case "name":
			less, tied = result[i].Name < result[j].Name, result[i].Name == result[j].Name
		case "jobType":
			less, tied = result[i].JobType < result[j].JobType, result[i].JobType == result[j].JobType
		default:
			return false
		}

		if tied {
			// JobID is this table's unique key (classificationJobKeyFn); breaking
			// ties on it keeps a total order so the offset-based page.NewHMAC cursor
			// can't drop or duplicate jobs that tie on the requested attribute.
			return result[i].JobID < result[j].JobID
		}

		if desc {
			return !less
		}

		return less
	})
}

// ListClassificationJobs returns summaries of jobs matching filterCriteria,
// paginated by maxResults/nextToken. filterCriteria mirrors the wire shape
// of types.ListJobsFilterCriteria: {"includes": [...], "excludes": [...]},
// each term {"key", "comparator", "values"}. Only EQ/NE comparators are
// emulated (the same reduced-scope emulation as ListFindings' criteria
// matching) -- GT/GTE/LT/LTE/CONTAINS/STARTS_WITH terms are treated as
// non-filtering, not as errors.
func (b *InMemoryBackend) ListClassificationJobs(
	filterCriteria map[string]any, sortBy *ListJobsSortCriteria, maxResults int, nextToken string,
) ([]*ClassificationJobSummary, string, error) {
	return listPaginated(
		b, "ListClassificationJobs", b.classificationJobs.All(),
		func(job *ClassificationJob) (*ClassificationJobSummary, bool) {
			if !matchesJobCriteria(job, filterCriteria) {
				return nil, false
			}

			return jobToSummary(job), true
		},
		func(result []*ClassificationJobSummary) { sortJobSummaries(result, sortBy) },
		nextToken, maxResults,
	)
}

// jobToSummary projects a ClassificationJob onto its JobSummary wire shape.
// bucketCriteria/bucketDefinitions are extracted from the stored
// s3JobDefinition instead of being independently typed: the real API
// flattens exactly one of those two mutually-exclusive keys from the job's
// S3JobDefinition onto the list-view JobSummary, and since s3JobDefinition
// is stored as the client's original request payload, passing the same
// sub-values through preserves whatever nested shape the client sent.
func jobToSummary(job *ClassificationJob) *ClassificationJobSummary {
	summary := &ClassificationJobSummary{
		JobID:              job.JobID,
		Name:               job.Name,
		Description:        job.Description,
		JobType:            job.JobType,
		JobStatus:          job.JobStatus,
		CreatedAt:          job.CreatedAt,
		LastRunTime:        job.LastRunTime,
		LastRunErrorStatus: job.LastRunErrorStatus,
		UserPausedDetails:  job.UserPausedDetails,
		Tags:               maps.Clone(job.Tags),
	}

	if job.S3JobDefinition != nil {
		summary.BucketCriteria = job.S3JobDefinition["bucketCriteria"]
		summary.BucketDefinitions = job.S3JobDefinition["bucketDefinitions"]
	}

	return summary
}

func jobFieldValue(job *ClassificationJob, key string) string {
	switch key {
	case "jobType":
		return job.JobType
	case "jobStatus":
		return job.JobStatus
	case "name":
		return job.Name
	case "createdAt":
		return job.CreatedAt.Format(time.RFC3339)
	}

	return ""
}

func matchesJobTerm(job *ClassificationJob, term map[string]any) bool {
	key, _ := term["key"].(string)
	comparator, _ := term["comparator"].(string)
	values, _ := term["values"].([]any)
	fVal := jobFieldValue(job, key)

	switch comparator {
	case "NE":
		for _, v := range values {
			if s, ok := v.(string); ok && s == fVal {
				return false
			}
		}

		return true
	case "EQ", "":
		for _, v := range values {
			if s, ok := v.(string); ok && s == fVal {
				return true
			}
		}

		return false
	default:
		return true
	}
}

func matchesJobCriteria(job *ClassificationJob, criteria map[string]any) bool {
	if len(criteria) == 0 {
		return true
	}

	if includes, ok := criteria["includes"].([]any); ok {
		for _, raw := range includes {
			term, tOk := raw.(map[string]any)
			if tOk && !matchesJobTerm(job, term) {
				return false
			}
		}
	}

	if excludes, ok := criteria["excludes"].([]any); ok {
		for _, raw := range excludes {
			term, tOk := raw.(map[string]any)
			if tOk && matchesJobTerm(job, term) {
				return false
			}
		}
	}

	return true
}

// jobPauseWindow is how long a USER_PAUSED job/run is kept before it expires
// and is cancelled, matching UserPausedDetails.jobExpiresAt's documented
// 30-day pause window.
const jobPauseWindow = 30 * 24 * time.Hour

// validJobStatusTransitions mirrors the three "valid only if the job's
// current status is ..." preconditions on UpdateClassificationJobInput.
// JobStatus's doc comment (api_op_UpdateClassificationJob.go:37-58): the
// requested status maps to the set of current statuses it may be applied
// from. A requested status outside this map (e.g. COMPLETE, IDLE, PAUSED)
// isn't one of the doc's three settable "Valid values", so it's rejected too.
var validJobStatusTransitions = map[string]map[string]bool{ //nolint:gochecknoglobals // read-only lookup data
	jobStatusCancelled:  {jobStatusIdle: true, statusPaused: true, jobStatusRunning: true, jobStatusUserPaused: true},
	jobStatusRunning:    {jobStatusUserPaused: true},
	jobStatusUserPaused: {jobStatusIdle: true, statusPaused: true, jobStatusRunning: true},
}

// UpdateClassificationJob updates a job's status. Transitioning to
// USER_PAUSED populates UserPausedDetails (present only in that state, per
// the real DescribeClassificationJobOutput/JobSummary shapes); transitioning
// away from it clears UserPausedDetails again.
func (b *InMemoryBackend) UpdateClassificationJob(jobID, status string) error {
	b.mu.Lock("UpdateClassificationJob")
	defer b.mu.Unlock()

	job, ok := b.classificationJobs.Get(jobID)
	if !ok {
		return ErrClassificationJobNotFound
	}

	allowedFrom, recognized := validJobStatusTransitions[status]
	if !recognized {
		return ErrValidation
	}

	if !allowedFrom[job.JobStatus] {
		return ErrJobStatusTransition
	}

	job.JobStatus = status

	if status == jobStatusUserPaused {
		now := time.Now().UTC()
		expires := now.Add(jobPauseWindow)
		job.UserPausedDetails = &JobUserPausedDetails{JobPausedAt: &now, JobExpiresAt: &expires}
	} else {
		job.UserPausedDetails = nil
	}

	return nil
}

// GetClassificationExportConfiguration returns the export config.
func (b *InMemoryBackend) GetClassificationExportConfiguration() (*ClassificationExportConfig, error) {
	b.mu.RLock("GetClassificationExportConfiguration")
	defer b.mu.RUnlock()

	if b.classExportConfig == nil {
		return &ClassificationExportConfig{}, nil
	}

	cp := *b.classExportConfig

	return &cp, nil
}

// PutClassificationExportConfiguration stores the export config.
func (b *InMemoryBackend) PutClassificationExportConfiguration(cfg *ClassificationExportConfig) error {
	b.mu.Lock("PutClassificationExportConfiguration")
	defer b.mu.Unlock()

	if cfg == nil {
		b.classExportConfig = nil

		return nil
	}

	cp := *cfg
	b.classExportConfig = &cp

	return nil
}

const defaultScopeName = "Default classification scope"

func (b *InMemoryBackend) ensureDefaultScope() {
	if b.classScopes.Len() > 0 {
		return
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	b.classScopes.Put(&ClassificationScope{
		ID:        id,
		Name:      defaultScopeName,
		S3:        &ClassificationScopeS3{},
		CreatedAt: now,
		UpdatedAt: now,
	})
}

// GetClassificationScope returns a classification scope by ID.
func (b *InMemoryBackend) GetClassificationScope(scopeID string) (*ClassificationScope, error) {
	b.mu.Lock("GetClassificationScope")
	defer b.mu.Unlock()

	b.ensureDefaultScope()

	scope, ok := b.classScopes.Get(scopeID)
	if !ok {
		return nil, ErrClassificationScopeNotFound
	}

	cp := *scope

	return &cp, nil
}

// ListClassificationScopes returns all classification scopes.
func (b *InMemoryBackend) ListClassificationScopes() ([]*ClassificationScopeSummary, error) {
	b.mu.Lock("ListClassificationScopes")
	defer b.mu.Unlock()

	b.ensureDefaultScope()

	scopes := b.classScopes.All()
	result := make([]*ClassificationScopeSummary, 0, len(scopes))

	for _, s := range scopes {
		result = append(result, &ClassificationScopeSummary{ID: s.ID, Name: s.Name})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })

	return result, nil
}

// UpdateClassificationScope applies an ADD/REMOVE/REPLACE change to a
// scope's excluded-bucket list. Real AWS
// (types.S3ClassificationScopeExclusionUpdate) never accepts a full
// replacement list implicitly -- Operation is a required discriminator, so
// wholesale-replacing the stored Excludes with whatever the request carries
// would silently drop every bucket name an ADD/REMOVE call didn't mention
// (gopherstack-c8ge).
func (b *InMemoryBackend) UpdateClassificationScope(scopeID string, s3 *ClassificationScopeS3Update) error {
	b.mu.Lock("UpdateClassificationScope")
	defer b.mu.Unlock()

	scope, ok := b.classScopes.Get(scopeID)
	if !ok {
		return ErrClassificationScopeNotFound
	}

	if s3 != nil && s3.Excludes != nil {
		if scope.S3 == nil {
			scope.S3 = &ClassificationScopeS3{}
		}
		scope.S3.Excludes = mergeClassificationScopeExclusion(scope.S3.Excludes, s3.Excludes)
	}

	scope.UpdatedAt = time.Now().UTC()

	return nil
}

// mergeClassificationScopeExclusion applies upd's ADD/REMOVE/REPLACE
// operation to existing, returning the resulting bucket-name list. An
// unrecognized or empty Operation falls back to REPLACE.
func mergeClassificationScopeExclusion(
	existing *ClassificationScopeS3Exclusion,
	upd *ClassificationScopeS3ExclusionUpdate,
) *ClassificationScopeS3Exclusion {
	var current []string
	if existing != nil {
		current = existing.BucketNames
	}

	switch upd.Operation {
	case "ADD":
		seen := make(map[string]struct{}, len(current))
		merged := append([]string(nil), current...)
		for _, n := range current {
			seen[n] = struct{}{}
		}
		for _, n := range upd.BucketNames {
			if _, ok := seen[n]; !ok {
				seen[n] = struct{}{}
				merged = append(merged, n)
			}
		}

		return &ClassificationScopeS3Exclusion{BucketNames: merged}

	case "REMOVE":
		remove := make(map[string]struct{}, len(upd.BucketNames))
		for _, n := range upd.BucketNames {
			remove[n] = struct{}{}
		}
		kept := make([]string, 0, len(current))
		for _, n := range current {
			if _, ok := remove[n]; !ok {
				kept = append(kept, n)
			}
		}

		return &ClassificationScopeS3Exclusion{BucketNames: kept}

	default: // "REPLACE" and anything unrecognized.
		return &ClassificationScopeS3Exclusion{BucketNames: append([]string(nil), upd.BucketNames...)}
	}
}
