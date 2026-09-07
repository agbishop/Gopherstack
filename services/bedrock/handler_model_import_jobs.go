package bedrock

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// modelDataSourceWire is the wire shape of the real ModelDataSource union,
// whose sole member today is "s3DataSource" (types.ModelDataSourceMemberS3DataSource).
type modelDataSourceWire struct {
	S3DataSource *s3DataSourceWire `json:"s3DataSource,omitempty"`
}

type s3DataSourceWire struct {
	S3Uri string `json:"s3Uri"`
}

// createModelImportJobInput is the parsed request body for CreateModelImportJob.
type createModelImportJobInput struct {
	ModelDataSource   *modelDataSourceWire `json:"modelDataSource"`
	JobName           string               `json:"jobName"`
	ImportedModelName string               `json:"importedModelName"`
	RoleArn           string               `json:"roleArn"`
	Tags              []Tag                `json:"jobTags,omitempty"`
}

func (h *Handler) handleCreateModelImportJob(c *echo.Context) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return c.JSON(
			http.StatusInternalServerError,
			errorResponse("InternalServerException", "internal server error"),
		)
	}

	in, parseErr := parseBody[createModelImportJobInput](body)
	if parseErr != nil {
		return c.JSON(
			http.StatusBadRequest,
			errorResponse("ValidationException", "invalid request body"),
		)
	}

	var s3Uri string
	if in.ModelDataSource != nil && in.ModelDataSource.S3DataSource != nil {
		s3Uri = in.ModelDataSource.S3DataSource.S3Uri
	}

	job, opErr := h.Backend.CreateModelImportJob(in.JobName, in.ImportedModelName, in.RoleArn, s3Uri, in.Tags)
	if opErr != nil {
		return h.writeError(c, opErr)
	}

	return c.JSON(http.StatusCreated, modelImportJobToOutput(job))
}

// parseListModelImportJobsQuery builds the backend filter/sort/pagination
// input from the real ListModelImportJobs query-string bindings.
//
//nolint:dupl // mirrors sibling List*Query parsers over a distinct filter set.
func parseListModelImportJobsQuery(c *echo.Context) *ListModelImportJobsInput {
	q := c.Request().URL.Query()

	maxResults, _ := strconv.ParseInt(q.Get("maxResults"), 10, 32)

	in := &ListModelImportJobsInput{
		StatusEquals: q.Get("statusEquals"),
		NameContains: q.Get("nameContains"),
		SortBy:       q.Get("sortBy"),
		SortOrder:    q.Get("sortOrder"),
		NextToken:    q.Get("nextToken"),
		MaxResults:   int32(maxResults),
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

func (h *Handler) handleListModelImportJobs(c *echo.Context) error {
	jobs, nextToken := h.Backend.ListModelImportJobs(parseListModelImportJobsQuery(c))
	summaries := make([]map[string]any, 0, len(jobs))

	for _, j := range jobs {
		summaries = append(summaries, modelImportJobToSummary(j))
	}

	resp := map[string]any{"modelImportJobSummaries": summaries}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

// modelImportJobToSummary mirrors types.ModelImportJobSummary: creationTime,
// jobArn, jobName, status, endTime, importedModelArn, importedModelName,
// lastModifiedTime (bedrock@v1.66.4 types/types.go:5479-5514). No roleArn,
// modelDataSource or tags -- those are GetModelImportJob/CreateModelImportJob-only.
func modelImportJobToSummary(j *ModelImportJob) map[string]any {
	out := map[string]any{
		keyJobArn:           j.JobArn,
		keyJobName:          j.JobName,
		"importedModelArn":  j.ImportedModelArn,
		"importedModelName": j.ImportedModelName,
		keyStatus:           j.Status,
		keyCreationTime:     j.CreationTime.Format(time.RFC3339),
		keyLastModifiedTime: j.LastModifiedTime.Format(time.RFC3339),
	}

	if j.EndTime != nil {
		out["endTime"] = j.EndTime.Format(time.RFC3339)
	}

	return out
}

func (h *Handler) handleGetModelImportJob(c *echo.Context, jobARN string) error {
	job, err := h.Backend.GetModelImportJob(jobARN)
	if err != nil {
		return h.writeError(c, err)
	}

	return c.JSON(http.StatusOK, modelImportJobToOutput(job))
}

func modelImportJobToOutput(j *ModelImportJob) map[string]any {
	out := map[string]any{
		keyJobArn:           j.JobArn,
		keyJobName:          j.JobName,
		"importedModelArn":  j.ImportedModelArn,
		"importedModelName": j.ImportedModelName,
		keyRoleArn:          j.RoleArn,
		keyStatus:           j.Status,
		keyCreationTime:     j.CreationTime.Format(time.RFC3339),
		keyLastModifiedTime: j.LastModifiedTime.Format(time.RFC3339),
	}

	if j.ModelDataSourceS3 != "" {
		out["modelDataSource"] = modelDataSourceWire{S3DataSource: &s3DataSourceWire{S3Uri: j.ModelDataSourceS3}}
	}

	if j.EndTime != nil {
		out["endTime"] = j.EndTime.Format(time.RFC3339)
	}

	if len(j.Tags) > 0 {
		out["tags"] = j.Tags
	}

	return out
}

// importedModelToWire builds the real GetImportedModelOutput / ImportedModelSummary
// shape: modelArn, modelName, jobArn, jobName, creationTime, and (when known)
// modelDataSource. gopherstack previously invented a "status" field (not
// present on either real shape -- ImportedModel has no lifecycle status of its
// own, only the ModelImportJob that produced it does) and used "createdAt"
// instead of the real "creationTime" key.
func importedModelToWire(j *ModelImportJob) map[string]any {
	out := map[string]any{
		keyModelArn:     j.ImportedModelArn,
		"modelName":     j.ImportedModelName,
		"jobArn":        j.JobArn,
		"jobName":       j.JobName,
		keyCreationTime: j.CreationTime.Format(time.RFC3339),
	}

	if j.ModelDataSourceS3 != "" {
		out["modelDataSource"] = modelDataSourceWire{S3DataSource: &s3DataSourceWire{S3Uri: j.ModelDataSourceS3}}
	}

	return out
}

func (h *Handler) handleGetImportedModel(c *echo.Context, modelARN string) error {
	job, err := h.Backend.GetImportedModel(modelARN)
	if err != nil {
		return h.writeError(c, err)
	}

	return c.JSON(http.StatusOK, importedModelToWire(job))
}

// parseListImportedModelsQuery builds the backend filter/pagination input
// from the real ListImportedModels query-string bindings.
func parseListImportedModelsQuery(c *echo.Context) *ListImportedModelsInput {
	q := c.Request().URL.Query()

	maxResults, _ := strconv.ParseInt(q.Get("maxResults"), 10, 32)

	in := &ListImportedModelsInput{
		NameContains: q.Get("nameContains"),
		NextToken:    q.Get("nextToken"),
		MaxResults:   int32(maxResults),
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

func (h *Handler) handleListImportedModels(c *echo.Context) error {
	models, nextToken := h.Backend.ListImportedModels(parseListImportedModelsQuery(c))

	summaries := make([]map[string]any, 0, len(models))
	for _, m := range models {
		summaries = append(summaries, importedModelToWire(m))
	}

	resp := map[string]any{"modelSummaries": summaries}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleDeleteImportedModel(c *echo.Context, modelARN string) error {
	if err := h.Backend.DeleteImportedModel(modelARN); err != nil {
		return h.writeError(c, err)
	}

	return c.NoContent(http.StatusOK)
}
