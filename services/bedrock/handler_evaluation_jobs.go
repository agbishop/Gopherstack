package bedrock

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
)

func extractEvaluationJobOperation(path, method string) (string, bool) {
	switch {
	case path == evaluationJobsBatchDelete && method == http.MethodPost:
		return "BatchDeleteEvaluationJob", true
	case path == evaluationJobsPrefix && method == http.MethodPost:
		return "CreateEvaluationJob", true
	case path == evaluationJobsPrefix && method == http.MethodGet:
		return "ListEvaluationJobs", true
	case strings.HasPrefix(path, evaluationJobsPrefix+"/") && method == http.MethodGet:
		return "GetEvaluationJob", true
	case strings.HasPrefix(path, evaluationJobSingularPrefix+"/") &&
		strings.HasSuffix(path, "/stop") && method == http.MethodPost:
		return "StopEvaluationJob", true
	default:
		return "", false
	}
}

func (h *Handler) routeEvaluationJob(
	c *echo.Context,
	path, method string,
	body []byte,
) (bool, error) {
	switch {
	case path == evaluationJobsBatchDelete && method == http.MethodPost:
		return true, h.handleBatchDeleteEvaluationJob(c, body)
	case path == evaluationJobsPrefix && method == http.MethodPost:
		return true, h.handleCreateEvaluationJob(c, body)
	case path == evaluationJobsPrefix && method == http.MethodGet:
		return true, h.handleListEvaluationJobs(c)
	case strings.HasPrefix(path, evaluationJobsPrefix+"/") && method == http.MethodGet:
		jobARN, _ := url.PathUnescape(strings.TrimPrefix(path, evaluationJobsPrefix+"/"))

		return true, h.handleGetEvaluationJob(c, jobARN)
	case strings.HasPrefix(path, evaluationJobSingularPrefix+"/") &&
		strings.HasSuffix(path, "/stop") && method == http.MethodPost:
		rest := strings.TrimPrefix(path, evaluationJobSingularPrefix+"/")
		jobARN, _ := url.PathUnescape(strings.TrimSuffix(rest, "/stop"))

		return true, h.handleStopEvaluationJob(c, jobARN)
	default:
		return false, nil
	}
}

type createEvaluationJobInput struct {
	EvaluationConfig *EvaluationConfig          `json:"evaluationConfig,omitempty"`
	InferenceConfig  *EvaluationInferenceConfig `json:"inferenceConfig,omitempty"`
	OutputDataConfig *outputDataConfigInput     `json:"outputDataConfig,omitempty"`
	JobName          string                     `json:"jobName"`
	JobDescription   string                     `json:"jobDescription,omitempty"`
	RoleArn          string                     `json:"roleArn,omitempty"`
	ApplicationType  string                     `json:"applicationType,omitempty"`
	Tags             []Tag                      `json:"jobTags,omitempty"`
}

type createEvaluationJobOutput struct {
	JobArn string `json:"jobArn"`
}

func (h *Handler) handleCreateEvaluationJob(c *echo.Context, body []byte) error {
	in, err := parseBody[createEvaluationJobInput](body)
	if err != nil {
		return c.JSON(
			http.StatusBadRequest,
			errorResponse("ValidationException", "invalid request body"),
		)
	}

	opts := &CreateEvaluationJobInput{
		JobDescription:   in.JobDescription,
		RoleArn:          in.RoleArn,
		ApplicationType:  in.ApplicationType,
		EvaluationConfig: in.EvaluationConfig,
		InferenceConfig:  in.InferenceConfig,
	}
	if in.OutputDataConfig != nil {
		opts.OutputDataConfig = OutputDataConfig{S3Uri: in.OutputDataConfig.S3Uri}
	}

	job, opErr := h.Backend.CreateEvaluationJob(in.JobName, in.Tags, opts)
	if opErr != nil {
		return h.writeError(c, opErr)
	}

	return c.JSON(http.StatusCreated, createEvaluationJobOutput{JobArn: job.JobArn})
}

type batchDeleteEvaluationJobInput struct {
	JobIdentifiers []string `json:"jobIdentifiers"`
}

type batchDeleteEvaluationJobOutput struct {
	Errors         []BatchDeleteEvaluationJobError `json:"errors"`
	EvaluationJobs []BatchDeleteEvaluationJobItem  `json:"evaluationJobs"`
}

func (h *Handler) handleBatchDeleteEvaluationJob(c *echo.Context, body []byte) error {
	in, err := parseBody[batchDeleteEvaluationJobInput](body)
	if err != nil {
		return c.JSON(
			http.StatusBadRequest,
			errorResponse("ValidationException", "invalid request body"),
		)
	}

	errs, deleted, opErr := h.Backend.BatchDeleteEvaluationJob(in.JobIdentifiers)
	if opErr != nil {
		return h.writeError(c, opErr)
	}

	return c.JSON(
		http.StatusOK,
		batchDeleteEvaluationJobOutput{Errors: errs, EvaluationJobs: deleted},
	)
}

func (h *Handler) handleGetEvaluationJob(c *echo.Context, jobARN string) error {
	job, err := h.Backend.GetEvaluationJob(jobARN)
	if err != nil {
		return h.writeError(c, err)
	}

	// roleArn/inferenceConfig/evaluationConfig/outputDataConfig are all
	// required on GetEvaluationJobOutput (bedrock@v1.66.4
	// api_op_GetEvaluationJob.go:51-90); outputDataConfig was previously not
	// wired at all (no input parsing, no model field, no response key) --
	// entirely absent regardless of what a real client sent on Create.
	resp := map[string]any{
		keyJobArn:           job.JobArn,
		keyJobName:          job.JobName,
		keyStatus:           job.Status,
		keyCreationTime:     job.CreationTime.Format(time.RFC3339),
		keyLastModifiedTime: job.LastModifiedTime.Format(time.RFC3339),
		keyRoleArn:          job.RoleArn,
		"outputDataConfig":  outputDataConfigOutput{S3Uri: job.OutputDataConfig.S3Uri},
	}
	if job.ApplicationType != "" {
		resp["applicationType"] = job.ApplicationType
	}
	if job.JobDescription != "" {
		resp["jobDescription"] = job.JobDescription
	}
	if job.InferenceConfig != nil {
		resp["inferenceConfig"] = job.InferenceConfig
	}
	if job.EvaluationConfig != nil {
		resp["evaluationConfig"] = job.EvaluationConfig
		// JobType (required, api_op_GetEvaluationJob.go:72) is derivable now
		// that EvaluationConfig's union is modeled: real AWS reads it from
		// which variant the caller supplied on Create (types/enums.go:548-551).
		if jt := job.EvaluationConfig.JobType(); jt != "" {
			resp["jobType"] = jt
		}
	}

	return c.JSON(http.StatusOK, resp)
}

// parseListEvaluationJobsQuery builds the backend filter/sort/pagination input
// from the real ListEvaluationJobs query-string bindings (nameContains,
// statusEquals, applicationTypeEquals, creationTimeAfter/Before, sortBy,
// sortOrder, nextToken).
//
//nolint:dupl // mirrors sibling List*Query parsers over a distinct filter set.
func parseListEvaluationJobsQuery(c *echo.Context) *ListEvaluationJobsInput {
	q := c.Request().URL.Query()

	maxResults, _ := strconv.ParseInt(q.Get("maxResults"), 10, 32)

	in := &ListEvaluationJobsInput{
		StatusEquals:          q.Get("statusEquals"),
		ApplicationTypeEquals: q.Get("applicationTypeEquals"),
		NameContains:          q.Get("nameContains"),
		SortBy:                q.Get("sortBy"),
		SortOrder:             q.Get("sortOrder"),
		NextToken:             q.Get("nextToken"),
		MaxResults:            int32(maxResults),
	}

	if v := q.Get("creationTimeAfter"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			in.CreationTimeAfter = &t
		}
	}

	if v := q.Get("creationTimeBefore"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			in.CreationTimeBefore = &t
		}
	}

	return in
}

func (h *Handler) handleListEvaluationJobs(c *echo.Context) error {
	jobs, outToken := h.Backend.ListEvaluationJobs(parseListEvaluationJobsQuery(c))
	summaries := make([]map[string]any, 0, len(jobs))

	for _, j := range jobs {
		summary := map[string]any{
			keyJobArn:           j.JobArn,
			keyJobName:          j.JobName,
			keyStatus:           j.Status,
			keyCreationTime:     j.CreationTime.Format(time.RFC3339),
			keyLastModifiedTime: j.LastModifiedTime.Format(time.RFC3339),
		}
		if j.ApplicationType != "" {
			summary["applicationType"] = j.ApplicationType
		}

		summaries = append(summaries, summary)
	}

	resp := map[string]any{"jobSummaries": summaries}
	if outToken != "" {
		resp["nextToken"] = outToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleStopEvaluationJob(c *echo.Context, jobARN string) error {
	if err := h.Backend.StopEvaluationJob(jobARN); err != nil {
		return h.writeError(c, err)
	}

	return c.NoContent(http.StatusOK)
}
