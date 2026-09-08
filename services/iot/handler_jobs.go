package iot

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

func (h *Handler) handleAssociateTargetsWithJob(c *echo.Context) error {
	// Path: /jobs/{jobId}/targets
	after := strings.TrimPrefix(c.Request().URL.Path, "/jobs/")
	jobID, _, _ := strings.Cut(after, "/")

	var body struct {
		Comment     string   `json:"comment"`
		NamespaceID string   `json:"namespaceId"`
		Targets     []string `json:"targets"`
	}

	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil &&
		!errors.Is(err, io.EOF) {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	out, err := h.Backend.AssociateTargetsWithJob(&AssociateTargetsWithJobInput{
		JobID:       jobID,
		Targets:     body.Targets,
		Comment:     body.Comment,
		NamespaceID: body.NamespaceID,
	})
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, out)
}

// jobExecutionSummaryWire is the real wire shape of types.JobExecutionSummary
// (confirmed against awsRestjson1_deserializeDocumentJobExecutionSummary in
// aws-sdk-go-v2/service/iot@v1.77.4), nested inside both
// JobExecutionSummaryForJob and JobExecutionSummaryForThing below.
type jobExecutionSummaryWire struct {
	Status          JobExecutionStatus `json:"status,omitempty"`
	ExecutionNumber int64              `json:"executionNumber,omitempty"`
	QueuedAt        float64            `json:"queuedAt,omitempty"`
	StartedAt       float64            `json:"startedAt,omitempty"`
	LastUpdatedAt   float64            `json:"lastUpdatedAt,omitempty"`
}

func toJobExecutionSummaryWire(e *JobExecution) jobExecutionSummaryWire {
	return jobExecutionSummaryWire{
		ExecutionNumber: e.ExecutionNumber,
		QueuedAt:        e.QueuedAt,
		StartedAt:       e.StartedAt,
		LastUpdatedAt:   e.LastUpdatedAt,
		Status:          e.Status,
	}
}

// handleListJobExecutionsForJob: real AWS's
// ListJobExecutionsForJobOutput.executionSummaries is
// []JobExecutionSummaryForJob{ThingArn, JobExecutionSummary{...}}
// (awsRestjson1_deserializeDocumentJobExecutionSummaryForJob) — a nested
// shape with "thingArn", not a flat "jobId"/"thingName"/"status".
func (h *Handler) handleListJobExecutionsForJob(c *echo.Context) error {
	// GET /jobs/{jobId}/things
	trimmed := strings.TrimPrefix(c.Request().URL.Path, "/jobs/")
	jobID := strings.TrimSuffix(trimmed, "/things")
	execs := h.Backend.ListJobExecutionsForJob(jobID)
	summaries := make([]map[string]any, len(execs))

	for i, e := range execs {
		summaries[i] = map[string]any{
			"thingArn":            h.Backend.ThingARN(e.ThingName),
			"jobExecutionSummary": toJobExecutionSummaryWire(e),
		}
	}

	return c.JSON(http.StatusOK, map[string]any{"executionSummaries": summaries})
}

// handleListJobExecutionsForThing: same nested-shape fix as
// handleListJobExecutionsForJob, for the sibling
// JobExecutionSummaryForThing{JobId, JobExecutionSummary{...}}
// (awsRestjson1_deserializeDocumentJobExecutionSummaryForThing).
func (h *Handler) handleListJobExecutionsForThing(c *echo.Context) error {
	// GET /things/{thingName}/jobs
	thingName := extractThingName(c.Request().URL.Path)
	execs := h.Backend.ListJobExecutionsForThing(thingName)
	summaries := make([]map[string]any, len(execs))

	for i, e := range execs {
		summaries[i] = map[string]any{
			"jobId":               e.JobID,
			"jobExecutionSummary": toJobExecutionSummaryWire(e),
		}
	}

	return c.JSON(http.StatusOK, map[string]any{"executionSummaries": summaries})
}

func resolveJobOps(path, method string) string {
	if op := resolveJobExecutionSubPathOps(path, method); op != unknownOperation {
		return op
	}

	return resolveJobCrudOps(path, method)
}

// resolveJobExecutionSubPathOps resolves the /jobs/{jobId}/... sub-routes that must be
// checked before the generic per-job CRUD routing in resolveJobCrudOps.
//
// DescribeJobExecution/CancelJobExecution/DeleteJobExecution do NOT live
// here: real AWS IoT paths them under /things/{thingName}/jobs/{jobId}[...],
// not /jobs/{jobId}/things/{thingName} — see resolveThingJobExecutionOps in
// handler_routing.go (aws-sdk-go-v2/service/iot@v1.77.4's serializers.go
// http bindings).
//
// GetJobDocument's real path is /jobs/{jobId}/job-document, not
// /jobs/{jobId}/document (awsRestjson1_serializeOpGetJobDocument's
// httpbinding.SplitURI call).
func resolveJobExecutionSubPathOps(path, method string) string {
	switch {
	// GET /jobs/{jobId}/job-document → GetJobDocument
	case strings.HasPrefix(path, "/jobs/") && strings.HasSuffix(path, "/job-document") && method == http.MethodGet:
		return opGetJobDocument
	// PUT /jobs/{jobId}/cancel → CancelJob
	case strings.HasPrefix(path, "/jobs/") && strings.HasSuffix(path, "/cancel") && method == http.MethodPut:
		return opCancelJob
	// GET /jobs/{jobId}/things → ListJobExecutionsForJob (no thingName segment)
	case strings.HasPrefix(path, "/jobs/") && strings.HasSuffix(path, "/things") && method == http.MethodGet:
		return opListJobExecutionsForJob
	}

	return unknownOperation
}

// resolveJobCrudOps resolves the plain /jobs and /jobs/{jobId} CRUD routes.
//
// CreateJob matches on PUT, not POST: real AWS IoT's CreateJob is
// PUT /jobs/{jobId} (awsRestjson1_serializeOpCreateJob's request.Method
// assignment).
func resolveJobCrudOps(path, method string) string {
	switch {
	// GET /jobs → ListJobs
	case path == "/jobs" && method == http.MethodGet:
		return opListJobs
	// PUT /jobs/{jobId} → CreateJob
	case strings.HasPrefix(path, "/jobs/") && method == http.MethodPut:
		return opCreateJob
	// GET /jobs/{jobId} → DescribeJob
	case strings.HasPrefix(path, "/jobs/") && method == http.MethodGet:
		return opDescribeJob
	// PATCH /jobs/{jobId} → UpdateJob
	case strings.HasPrefix(path, "/jobs/") && method == http.MethodPatch:
		return opUpdateJob
	// DELETE /jobs/{jobId} → DeleteJob
	case strings.HasPrefix(path, "/jobs/") && method == http.MethodDelete:
		return opDeleteJob
	}

	return unknownOperation
}

// resolveJobTemplateOps resolves the /job-templates and /job-templates/{id}
// routes. CreateJobTemplate matches on PUT, not POST: real AWS IoT's
// CreateJobTemplate is PUT /job-templates/{jobTemplateId}
// (awsRestjson1_serializeOpCreateJobTemplate's request.Method assignment).
func resolveJobTemplateOps(path, method string) string {
	switch {
	case path == "/job-templates" && method == http.MethodGet:
		return opListJobTemplates
	case strings.HasPrefix(path, "/job-templates/") && method == http.MethodPut:
		return opCreateJobTemplate
	case strings.HasPrefix(path, "/job-templates/") && method == http.MethodGet:
		return opDescribeJobTemplate
	case strings.HasPrefix(path, "/job-templates/") && method == http.MethodDelete:
		return opDeleteJobTemplate
	}

	return unknownOperation
}

func (h *Handler) handleCreateJob(c *echo.Context) error {
	// jobId is in the path: POST /jobs/{jobId}
	jobID := strings.TrimPrefix(c.Request().URL.Path, "/jobs/")
	var input CreateJobInput
	if err := readBody(c, &input); err != nil {
		return err
	}
	input.JobID = jobID
	job, err := h.Backend.CreateJob(&input)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyJobID:       job.JobID,
		keyJobARN:      job.JobARN,
		keyDescription: job.Description,
	})
}

func (h *Handler) handleDescribeJob(c *echo.Context) error {
	jobID := strings.TrimPrefix(c.Request().URL.Path, "/jobs/")
	job, err := h.Backend.DescribeJob(jobID)
	if err != nil {
		return respondErr(c, err)
	}

	// Real DescribeJobOutput duplicates documentSource at the top level in
	// addition to inside the nested "job" object (verified against
	// awsRestjson1_deserializeOpDocumentDescribeJobOutput in
	// aws-sdk-go-v2/service/iot@v1.77.4).
	return c.JSON(http.StatusOK, map[string]any{
		"job":            job,
		"documentSource": job.DocumentSource,
	})
}

func (h *Handler) handleListJobs(c *echo.Context) error {
	jobs := h.Backend.ListJobs()
	summaries := make([]map[string]any, len(jobs))
	for i, j := range jobs {
		summaries[i] = map[string]any{
			keyJobID:          j.JobID,
			keyJobARN:         j.JobARN,
			"status":          j.Status,
			"targetSelection": j.TargetSelection,
			keyCreatedAt:      j.CreatedAt,
			keyLastUpdatedAt:  j.LastUpdatedAt,
		}
	}

	return c.JSON(http.StatusOK, map[string]any{"jobs": summaries})
}

func (h *Handler) handleUpdateJob(c *echo.Context) error {
	jobID := strings.TrimPrefix(c.Request().URL.Path, "/jobs/")
	var req struct {
		AbortConfig                *AbortConfig                `json:"abortConfig"`
		JobExecutionsRolloutConfig *JobExecutionsRolloutConfig `json:"jobExecutionsRolloutConfig"`
		TimeoutConfig              *TimeoutConfig              `json:"timeoutConfig"`
		JobExecutionsRetryConfig   *JobExecutionsRetryConfig   `json:"jobExecutionsRetryConfig"`
		PresignedURLConfig         *PresignedURLConfig         `json:"presignedUrlConfig"`
		Description                string                      `json:"description"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}
	if err := h.Backend.UpdateJob(jobID, &UpdateJobInput{
		Description:                req.Description,
		AbortConfig:                req.AbortConfig,
		JobExecutionsRolloutConfig: req.JobExecutionsRolloutConfig,
		TimeoutConfig:              req.TimeoutConfig,
		JobExecutionsRetryConfig:   req.JobExecutionsRetryConfig,
		PresignedURLConfig:         req.PresignedURLConfig,
	}); err != nil {
		return respondErr(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleDeleteJob(c *echo.Context) error {
	jobID := strings.TrimPrefix(c.Request().URL.Path, "/jobs/")
	if err := h.Backend.DeleteJob(jobID); err != nil {
		return respondErr(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleCancelJob(c *echo.Context) error {
	// PUT /jobs/{jobId}/cancel
	trimmed := strings.TrimPrefix(c.Request().URL.Path, "/jobs/")
	jobID := strings.TrimSuffix(trimmed, "/cancel")
	var req struct {
		Comment string `json:"comment"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}
	job, err := h.Backend.CancelJob(jobID, req.Comment)
	if err != nil {
		// CancelJob's own deserializeOpError switch declares no
		// InvalidStateTransitionException case -- InvalidRequestException is
		// the real type. Its ResourceNotFoundException case IS declared, so
		// only this sentinel needs the override.
		return respondAsInvalidRequest(c, err, ErrInvalidStateTransition)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyJobID:       job.JobID,
		keyJobARN:      job.JobARN,
		keyDescription: job.Description,
	})
}

func (h *Handler) handleGetJobDocument(c *echo.Context) error {
	// GET /jobs/{jobId}/job-document
	trimmed := strings.TrimPrefix(c.Request().URL.Path, "/jobs/")
	jobID := strings.TrimSuffix(trimmed, "/job-document")
	doc, err := h.Backend.GetJobDocument(jobID)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"document": doc})
}

// jobExecutionWire builds the real DescribeJobExecutionOutput.execution wire
// shape from the internal JobExecution domain type. This must never marshal
// *JobExecution directly: the domain type's ThingName field is
// internal-only storage (see its doc comment) and has no counterpart on the
// real wire, which instead carries "thingArn".
type jobExecutionWire struct {
	StatusDetails                    *JobExecutionStatusDetails `json:"statusDetails,omitempty"`
	JobID                            string                     `json:"jobId,omitempty"`
	ThingArn                         string                     `json:"thingArn,omitempty"`
	Status                           JobExecutionStatus         `json:"status,omitempty"`
	ExecutionNumber                  int64                      `json:"executionNumber,omitempty"`
	QueuedAt                         float64                    `json:"queuedAt,omitempty"`
	StartedAt                        float64                    `json:"startedAt,omitempty"`
	LastUpdatedAt                    float64                    `json:"lastUpdatedAt,omitempty"`
	ApproximateSecondsBeforeTimedOut int64                      `json:"approximateSecondsBeforeTimedOut,omitempty"`
	VersionNumber                    int64                      `json:"versionNumber,omitempty"`
	ForceCanceled                    bool                       `json:"forceCanceled,omitempty"`
}

func (h *Handler) toJobExecutionWire(e *JobExecution) jobExecutionWire {
	return jobExecutionWire{
		JobID:                            e.JobID,
		ThingArn:                         h.Backend.ThingARN(e.ThingName),
		Status:                           e.Status,
		ExecutionNumber:                  e.ExecutionNumber,
		QueuedAt:                         e.QueuedAt,
		StartedAt:                        e.StartedAt,
		LastUpdatedAt:                    e.LastUpdatedAt,
		ApproximateSecondsBeforeTimedOut: e.ApproximateSecondsBeforeTimedOut,
		VersionNumber:                    e.VersionNumber,
		ForceCanceled:                    e.ForceCanceled,
		StatusDetails:                    e.StatusDetails,
	}
}

func (h *Handler) handleDescribeJobExecution(c *echo.Context) error {
	// GET /things/{thingName}/jobs/{jobId}
	thingName, jobID := parseThingJobPath(c.Request().URL.Path)
	exec, err := h.Backend.DescribeJobExecution(jobID, thingName)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"execution": h.toJobExecutionWire(exec)})
}

func (h *Handler) handleCancelJobExecution(c *echo.Context) error {
	// PUT /things/{thingName}/jobs/{jobId}/cancel
	path := strings.TrimSuffix(c.Request().URL.Path, "/cancel")
	thingName, jobID := parseThingJobPath(path)

	var body struct {
		StatusDetails   map[string]string `json:"statusDetails"`
		ExpectedVersion int64             `json:"expectedVersion"`
		Force           bool              `json:"force"`
	}

	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	err := h.Backend.CancelJobExecution(jobID, thingName, CancelJobExecutionOptions{
		Force:           body.Force,
		StatusDetails:   body.StatusDetails,
		ExpectedVersion: body.ExpectedVersion,
	})
	if err != nil {
		return respondErr(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleDeleteJobExecution(c *echo.Context) error {
	// DELETE /things/{thingName}/jobs/{jobId}/executionNumber/{executionNumber}?force=<bool>
	path := c.Request().URL.Path
	if idx := strings.Index(path, "/executionNumber/"); idx != -1 {
		path = path[:idx]
	}

	thingName, jobID := parseThingJobPath(path)
	force := c.QueryParam("force") == "true"

	if err := h.Backend.DeleteJobExecution(jobID, thingName, force); err != nil {
		return respondErr(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// parseThingJobPath extracts thingName and jobId from the real AWS path
// shape /things/{thingName}/jobs/{jobId}[/...] (confirmed against
// aws-sdk-go-v2/service/iot@v1.77.4's serializers.go http bindings for
// DescribeJobExecution/CancelJobExecution/DeleteJobExecution -- NOT
// /jobs/{jobId}/things/{thingName}, which no real client ever sends).
func parseThingJobPath(path string) (string, string) {
	trimmed := strings.TrimPrefix(path, "/things/")
	parts := strings.SplitN(trimmed, "/jobs/", twoparts)
	if len(parts) == twoparts {
		return parts[0], strings.SplitN(parts[1], "/", twoparts)[0]
	}

	return "", ""
}

func (h *Handler) handleCreateJobTemplate(c *echo.Context) error {
	id := strings.TrimPrefix(c.Request().URL.Path, "/job-templates/")
	var input CreateJobTemplateInput
	if err := readBody(c, &input); err != nil {
		return err
	}
	input.JobTemplateID = id
	jt, err := h.Backend.CreateJobTemplate(&input)
	if err != nil {
		return respondAsConflictCode(c, err, ErrAlreadyExists, "ConflictException")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"jobTemplateId":  jt.JobTemplateID,
		"jobTemplateArn": jt.JobTemplateARN,
	})
}

func (h *Handler) handleDescribeJobTemplate(c *echo.Context) error {
	id := strings.TrimPrefix(c.Request().URL.Path, "/job-templates/")
	jt, err := h.Backend.DescribeJobTemplate(id)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, jt)
}

func (h *Handler) handleListJobTemplates(c *echo.Context) error {
	templates := h.Backend.ListJobTemplates()
	summaries := make([]map[string]any, len(templates))
	for i, jt := range templates {
		summaries[i] = map[string]any{
			"jobTemplateId":  jt.JobTemplateID,
			"jobTemplateArn": jt.JobTemplateARN,
			keyDescription:   jt.Description,
			keyCreatedAt:     jt.CreatedAt,
		}
	}

	return c.JSON(http.StatusOK, map[string]any{"jobTemplates": summaries})
}

func (h *Handler) handleDeleteJobTemplate(c *echo.Context) error {
	id := strings.TrimPrefix(c.Request().URL.Path, "/job-templates/")
	if err := h.Backend.DeleteJobTemplate(id); err != nil {
		return respondErr(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) dispatchJobOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opCreateJob:
		return true, h.handleCreateJob(c)
	case opDescribeJob:
		return true, h.handleDescribeJob(c)
	case opListJobs:
		return true, h.handleListJobs(c)
	case opUpdateJob:
		return true, h.handleUpdateJob(c)
	case opCancelJob:
		return true, h.handleCancelJob(c)
	case opDeleteJob:
		return true, h.handleDeleteJob(c)
	case opGetJobDocument:
		return true, h.handleGetJobDocument(c)
	case opDescribeJobExecution:
		return true, h.handleDescribeJobExecution(c)
	case opCancelJobExecution:
		return true, h.handleCancelJobExecution(c)
	case opDeleteJobExecution:
		return true, h.handleDeleteJobExecution(c)
	case opListJobExecutionsForJob:
		return true, h.handleListJobExecutionsForJob(c)
	case opListJobExecutionsForThing:
		return true, h.handleListJobExecutionsForThing(c)
	}

	return false, nil
}

func (h *Handler) dispatchJobTemplateOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opCreateJobTemplate:
		return true, h.handleCreateJobTemplate(c)
	case opDescribeJobTemplate:
		return true, h.handleDescribeJobTemplate(c)
	case opListJobTemplates:
		return true, h.handleListJobTemplates(c)
	case opDeleteJobTemplate:
		return true, h.handleDeleteJobTemplate(c)
	}

	return false, nil
}

// resolveJobExecutionPathOps resolves the job-execution listing endpoints.
func resolveJobExecutionPathOps(path, method string) string {
	switch {
	case strings.HasPrefix(path, "/jobs/") &&
		strings.HasSuffix(path, "/things") &&
		method == http.MethodGet:
		return opListJobExecutionsForJob
	case strings.HasPrefix(path, "/things/") &&
		strings.HasSuffix(path, "/jobs") &&
		method == http.MethodGet:
		return opListJobExecutionsForThing
	}

	return unknownOperation
}
