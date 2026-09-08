package bedrock

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// routeStubInvocationOps handles model invocation job operations. AWS uses the SINGULAR
// "/model-invocation-job" path for Create/Get/Stop and the PLURAL
// "/model-invocation-jobs" path only for List — see modelInvocationJobSingularPrefix.
func (h *Handler) routeStubInvocationOps(c *echo.Context, path, method string) (bool, error) {
	switch {
	case path == modelInvocationJobsPrefix && method == http.MethodGet:
		return true, h.handleListModelInvocationJobs(c)
	case path == modelInvocationJobSingularPrefix && method == http.MethodPost:
		return true, h.handleCreateModelInvocationJob(c)
	case strings.HasPrefix(path, modelInvocationJobSingularPrefix+"/") &&
		strings.HasSuffix(path, "/stop") && method == http.MethodPost:
		rest := strings.TrimPrefix(path, modelInvocationJobSingularPrefix+"/")
		jobARN, _ := url.PathUnescape(strings.TrimSuffix(rest, "/stop"))

		return true, h.handleStopModelInvocationJob(c, jobARN)
	case strings.HasPrefix(path, modelInvocationJobSingularPrefix+"/") && method == http.MethodGet:
		jobARN, _ := url.PathUnescape(strings.TrimPrefix(path, modelInvocationJobSingularPrefix+"/"))

		return true, h.handleGetModelInvocationJob(c, jobARN)
	}

	return false, nil
}

// extractModelInvocationJobOperation maps the ModelInvocationJob family. AWS uses the
// SINGULAR "model-invocation-job" path for Create/Get/Stop and the PLURAL
// "model-invocation-jobs" path only for List.
func extractModelInvocationJobOperation(path, method string) (string, bool) {
	switch {
	case path == modelInvocationJobsPrefix && method == http.MethodGet:
		return "ListModelInvocationJobs", true
	case path == modelInvocationJobSingularPrefix && method == http.MethodPost:
		return "CreateModelInvocationJob", true
	case strings.HasPrefix(path, modelInvocationJobSingularPrefix+"/") &&
		strings.HasSuffix(path, "/stop") && method == http.MethodPost:
		return "StopModelInvocationJob", true
	case strings.HasPrefix(path, modelInvocationJobSingularPrefix+"/") && method == http.MethodGet:
		return "GetModelInvocationJob", true
	default:
		return "", false
	}
}

type createModelInvocationJobInput struct {
	JobName          string         `json:"jobName"`
	ModelID          string         `json:"modelId"`
	RoleArn          string         `json:"roleArn"`
	InputDataConfig  map[string]any `json:"inputDataConfig,omitempty"`
	OutputDataConfig map[string]any `json:"outputDataConfig,omitempty"`
	ClientToken      string         `json:"clientRequestToken,omitempty"`
	Tags             []Tag          `json:"tags,omitempty"`
}

func (h *Handler) handleCreateModelInvocationJob(c *echo.Context) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse("InternalServerException", "internal server error"))
	}

	in, parseErr := parseBody[createModelInvocationJobInput](body)
	if parseErr != nil {
		return c.JSON(http.StatusBadRequest, errorResponse("ValidationException", "invalid request body"))
	}

	job, opErr := h.Backend.CreateModelInvocationJob(in.JobName, in.Tags, &CreateModelInvocationJobInput{
		RoleArn:          in.RoleArn,
		ModelID:          in.ModelID,
		InputDataConfig:  in.InputDataConfig,
		OutputDataConfig: in.OutputDataConfig,
		ClientToken:      in.ClientToken,
	})
	if opErr != nil {
		return h.writeError(c, opErr)
	}

	return c.JSON(http.StatusCreated, map[string]any{keyJobArn: job.JobArn})
}

// modelInvocationJobToSummary mirrors types.ModelInvocationJobSummary, which
// GetModelInvocationJobOutput shares field-for-field (bedrock@v1.66.4
// types/types.go:5592-5722 vs api_op_GetModelInvocationJob.go): jobArn,
// jobName, modelId, roleArn, inputDataConfig, outputDataConfig, submitTime,
// status, lastModifiedTime, endTime. Neither shape carries tags -- job.Tags
// is Create-only, surfaced through ListTagsForResource, and must not leak
// here. submitTime has no dedicated domain field; job.CreationTime already
// records submission time and is reused, since the real shape has no
// separate creationTime key at all.
func modelInvocationJobToSummary(j *ModelInvocationJob) map[string]any {
	out := map[string]any{
		keyJobArn:           j.JobArn,
		keyJobName:          j.JobName,
		keyModelID:          j.ModelID,
		keyRoleArn:          j.RoleArn,
		"inputDataConfig":   j.InputDataConfig,
		"outputDataConfig":  j.OutputDataConfig,
		keyStatus:           j.Status,
		"submitTime":        j.CreationTime.Format(time.RFC3339),
		keyLastModifiedTime: j.LastModifiedTime.Format(time.RFC3339),
	}

	if j.EndTime != nil {
		out["endTime"] = j.EndTime.Format(time.RFC3339)
	}

	return out
}

func (h *Handler) handleGetModelInvocationJob(c *echo.Context, jobARN string) error {
	job, err := h.Backend.GetModelInvocationJob(jobARN)
	if err != nil {
		return h.writeError(c, err)
	}

	return c.JSON(http.StatusOK, modelInvocationJobToSummary(job))
}

// parseListModelInvocationJobsQuery builds the backend filter/sort/pagination input from
// the real ListModelInvocationJobs query-string bindings (nameContains, statusEquals,
// sortBy, sortOrder, nextToken, submitTimeAfter, submitTimeBefore). The previous
// implementation discarded all of these — a disguised no-op, since the backend already
// implements the filter/sort logic in full.
//
//nolint:dupl // mirrors sibling List*Query parsers over a distinct filter set.
func parseListModelInvocationJobsQuery(c *echo.Context) *ListModelInvocationJobsInput {
	q := c.Request().URL.Query()

	maxResults, _ := strconv.ParseInt(q.Get("maxResults"), 10, 32)

	in := &ListModelInvocationJobsInput{
		StatusEquals: q.Get("statusEquals"),
		NameContains: q.Get("nameContains"),
		SortBy:       q.Get("sortBy"),
		SortOrder:    q.Get("sortOrder"),
		NextToken:    q.Get("nextToken"),
		MaxResults:   int32(maxResults),
	}

	if v := q.Get("submitTimeAfter"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			in.SubmitTimeAfter = &t
		}
	}

	if v := q.Get("submitTimeBefore"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			in.SubmitTimeBefore = &t
		}
	}

	return in
}

func (h *Handler) handleListModelInvocationJobs(c *echo.Context) error {
	jobs, outToken := h.Backend.ListModelInvocationJobs(parseListModelInvocationJobsQuery(c))
	summaries := make([]map[string]any, 0, len(jobs))

	for _, j := range jobs {
		summaries = append(summaries, modelInvocationJobToSummary(j))
	}

	resp := map[string]any{"invocationJobSummaries": summaries}
	if outToken != "" {
		resp["nextToken"] = outToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleStopModelInvocationJob(c *echo.Context, jobARN string) error {
	if err := h.Backend.StopModelInvocationJob(jobARN); err != nil {
		return h.writeError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}
