package bedrock

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
)

// extractAdvancedPromptOptimizationJobOperation matches
// CreateAdvancedPromptOptimizationJob, GetAdvancedPromptOptimizationJob,
// ListAdvancedPromptOptimizationJobs, StopAdvancedPromptOptimizationJob, and
// BatchDeleteAdvancedPromptOptimizationJob. The batch-delete path is checked
// first because it uses the SINGULAR "advanced-prompt-optimization-job"
// prefix (real AWS: POST /advanced-prompt-optimization-job/batch-delete),
// distinct from the PLURAL prefix every other op in this family uses --
// same bug class this campaign already fixed for StopEvaluationJob and
// CreateModelInvocationJob's singular/plural path splits (see handler.go's
// evaluationJobSingularPrefix/modelInvocationJobSingularPrefix comments).
func extractAdvancedPromptOptimizationJobOperation(path, method string) (string, bool) {
	switch {
	case path == advancedPromptOptimizationJobBatchDeletePath && method == http.MethodPost:
		return "BatchDeleteAdvancedPromptOptimizationJob", true
	case path == advancedPromptOptimizationJobsPrefix && method == http.MethodPost:
		return "CreateAdvancedPromptOptimizationJob", true
	case path == advancedPromptOptimizationJobsPrefix && method == http.MethodGet:
		return "ListAdvancedPromptOptimizationJobs", true
	case strings.HasPrefix(path, advancedPromptOptimizationJobsPrefix+"/") &&
		strings.HasSuffix(path, "/stop") && method == http.MethodPost:
		return "StopAdvancedPromptOptimizationJob", true
	case strings.HasPrefix(path, advancedPromptOptimizationJobsPrefix+"/") && method == http.MethodGet:
		return "GetAdvancedPromptOptimizationJob", true
	default:
		return "", false
	}
}

func (h *Handler) routeAdvancedPromptOptimizationJob(
	c *echo.Context, path, method string, body []byte,
) (bool, error) {
	switch {
	case path == advancedPromptOptimizationJobBatchDeletePath && method == http.MethodPost:
		return true, h.handleBatchDeleteAdvancedPromptOptimizationJob(c, body)
	case path == advancedPromptOptimizationJobsPrefix && method == http.MethodPost:
		return true, h.handleCreateAdvancedPromptOptimizationJob(c, body)
	case path == advancedPromptOptimizationJobsPrefix && method == http.MethodGet:
		return true, h.handleListAdvancedPromptOptimizationJobs(c)
	case strings.HasPrefix(path, advancedPromptOptimizationJobsPrefix+"/") &&
		strings.HasSuffix(path, "/stop") && method == http.MethodPost:
		rest := strings.TrimPrefix(path, advancedPromptOptimizationJobsPrefix+"/")
		id := decodePath(strings.TrimSuffix(rest, "/stop"))

		return true, h.handleStopAdvancedPromptOptimizationJob(c, id)
	case strings.HasPrefix(path, advancedPromptOptimizationJobsPrefix+"/") && method == http.MethodGet:
		id := decodePath(strings.TrimPrefix(path, advancedPromptOptimizationJobsPrefix+"/"))

		return true, h.handleGetAdvancedPromptOptimizationJob(c, id)
	default:
		return false, nil
	}
}

type createAdvancedPromptOptimizationJobInput struct {
	JobName             string                                 `json:"jobName"`
	JobDescription      string                                 `json:"jobDescription,omitempty"`
	EncryptionKeyArn    string                                 `json:"encryptionKeyArn,omitempty"`
	ClientToken         string                                 `json:"clientToken,omitempty"`
	InputConfig         AdvancedPromptOptimizationInputConfig  `json:"inputConfig"`
	OutputConfig        AdvancedPromptOptimizationOutputConfig `json:"outputConfig"`
	ModelConfigurations []ModelConfiguration                   `json:"modelConfigurations"`
	Tags                []Tag                                  `json:"tags,omitempty"`
}

type createAdvancedPromptOptimizationJobOutput struct {
	JobArn string `json:"jobArn"`
}

func (h *Handler) handleCreateAdvancedPromptOptimizationJob(c *echo.Context, body []byte) error {
	in, err := parseBody[createAdvancedPromptOptimizationJobInput](body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse("ValidationException", "invalid request body"))
	}

	job, opErr := h.Backend.CreateAdvancedPromptOptimizationJob(CreateAdvancedPromptOptimizationJobInput{
		JobName:             in.JobName,
		JobDescription:      in.JobDescription,
		EncryptionKeyArn:    in.EncryptionKeyArn,
		InputConfig:         in.InputConfig,
		OutputConfig:        in.OutputConfig,
		ModelConfigurations: in.ModelConfigurations,
		Tags:                in.Tags,
	})
	if opErr != nil {
		return h.writeError(c, opErr)
	}

	return c.JSON(http.StatusOK, createAdvancedPromptOptimizationJobOutput{JobArn: job.JobArn})
}

type advancedPromptOptimizationJobOutput struct {
	CreationTime        string                                 `json:"creationTime"`
	LastModifiedTime    string                                 `json:"lastModifiedTime,omitempty"`
	JobArn              string                                 `json:"jobArn"`
	JobName             string                                 `json:"jobName"`
	JobDescription      string                                 `json:"jobDescription,omitempty"`
	JobStatus           string                                 `json:"jobStatus"`
	EncryptionKeyArn    string                                 `json:"encryptionKeyArn,omitempty"`
	FailureMessage      string                                 `json:"failureMessage,omitempty"`
	InputConfig         AdvancedPromptOptimizationInputConfig  `json:"inputConfig"`
	OutputConfig        AdvancedPromptOptimizationOutputConfig `json:"outputConfig"`
	ModelConfigurations []ModelConfiguration                   `json:"modelConfigurations"`
}

func advancedPromptOptimizationJobToOutput(j *AdvancedPromptOptimizationJob) advancedPromptOptimizationJobOutput {
	out := advancedPromptOptimizationJobOutput{
		JobArn:              j.JobArn,
		JobName:             j.JobName,
		JobDescription:      j.JobDescription,
		JobStatus:           j.JobStatus,
		EncryptionKeyArn:    j.EncryptionKeyArn,
		FailureMessage:      j.FailureMessage,
		InputConfig:         j.InputConfig,
		OutputConfig:        j.OutputConfig,
		ModelConfigurations: j.ModelConfigurations,
		CreationTime:        j.CreationTime.Format(time.RFC3339),
	}
	if !j.LastModifiedTime.IsZero() {
		out.LastModifiedTime = j.LastModifiedTime.Format(time.RFC3339)
	}

	return out
}

func (h *Handler) handleGetAdvancedPromptOptimizationJob(c *echo.Context, id string) error {
	job, err := h.Backend.GetAdvancedPromptOptimizationJob(id)
	if err != nil {
		return h.writeError(c, err)
	}

	return c.JSON(http.StatusOK, advancedPromptOptimizationJobToOutput(job))
}

type listAdvancedPromptOptimizationJobsOutput struct {
	NextToken    string                                `json:"nextToken,omitempty"`
	JobSummaries []advancedPromptOptimizationJobOutput `json:"jobSummaries"`
}

func parseListAdvancedPromptOptimizationJobsQuery(c *echo.Context) *ListAdvancedPromptOptimizationJobsInput {
	q := c.Request().URL.Query()

	maxResults, _ := strconv.ParseInt(q.Get("maxResults"), 10, 32)

	in := &ListAdvancedPromptOptimizationJobsInput{
		SortBy:     q.Get("sortBy"),
		SortOrder:  q.Get("sortOrder"),
		NextToken:  q.Get("nextToken"),
		MaxResults: int32(maxResults),
	}

	return in
}

func (h *Handler) handleListAdvancedPromptOptimizationJobs(c *echo.Context) error {
	jobs, outToken := h.Backend.ListAdvancedPromptOptimizationJobs(parseListAdvancedPromptOptimizationJobsQuery(c))

	summaries := make([]advancedPromptOptimizationJobOutput, 0, len(jobs))
	for _, j := range jobs {
		summaries = append(summaries, advancedPromptOptimizationJobToOutput(j))
	}

	return c.JSON(http.StatusOK, listAdvancedPromptOptimizationJobsOutput{
		JobSummaries: summaries,
		NextToken:    outToken,
	})
}

func (h *Handler) handleStopAdvancedPromptOptimizationJob(c *echo.Context, id string) error {
	if err := h.Backend.StopAdvancedPromptOptimizationJob(id); err != nil {
		return h.writeError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

type batchDeleteAdvancedPromptOptimizationJobInput struct {
	JobIdentifiers []string `json:"jobIdentifiers"`
}

type batchDeleteAdvancedPromptOptimizationJobOutput struct {
	AdvancedPromptOptimizationJobs []BatchDeleteAdvancedPromptOptimizationJobItem  `json:"advancedPromptOptimizationJobs"`
	Errors                         []BatchDeleteAdvancedPromptOptimizationJobError `json:"errors"`
}

func (h *Handler) handleBatchDeleteAdvancedPromptOptimizationJob(c *echo.Context, body []byte) error {
	in, err := parseBody[batchDeleteAdvancedPromptOptimizationJobInput](body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse("ValidationException", "invalid request body"))
	}

	deleted, errs, opErr := h.Backend.BatchDeleteAdvancedPromptOptimizationJob(in.JobIdentifiers)
	if opErr != nil {
		return h.writeError(c, opErr)
	}

	return c.JSON(http.StatusAccepted, batchDeleteAdvancedPromptOptimizationJobOutput{
		AdvancedPromptOptimizationJobs: deleted,
		Errors:                         errs,
	})
}
