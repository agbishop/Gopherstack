package pinpoint

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

const (
	exportJobType = "EXPORT"
	importJobType = "IMPORT"
)

// extractExportJobsOp returns the export jobs collection operation name.
func extractExportJobsOp(method string) string {
	switch method {
	case http.MethodPost:
		return "CreateExportJob"
	case http.MethodGet:
		return "GetExportJobs"
	}

	return unknownOperation
}

// extractImportJobsOp returns the import jobs collection operation name.
func extractImportJobsOp(method string) string {
	switch method {
	case http.MethodPost:
		return "CreateImportJob"
	case http.MethodGet:
		return "GetImportJobs"
	}

	return unknownOperation
}

func (h *Handler) dispatchExportJobs(c *echo.Context, appID, jobID string) error {
	if jobID != "" {
		return h.handleGetExportJob(c, appID, jobID)
	}

	switch c.Request().Method {
	case http.MethodPost:
		return h.handleCreateExportJob(c, appID)
	case http.MethodGet:
		return h.handleGetExportJobs(c, appID)
	}

	return writeErrorResponse(c, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
}

func (h *Handler) dispatchImportJobs(c *echo.Context, appID, jobID string) error {
	if jobID != "" {
		return h.handleGetImportJob(c, appID, jobID)
	}

	switch c.Request().Method {
	case http.MethodPost:
		return h.handleCreateImportJob(c, appID)
	case http.MethodGet:
		return h.handleGetImportJobs(c, appID)
	}

	return writeErrorResponse(c, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
}

// handleCreateExportJob handles POST /v1/apps/{appId}/jobs/export.
func (h *Handler) handleCreateExportJob(c *echo.Context, appID string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "failed to read request body")
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	var req createExportJobRequest
	if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "invalid request body")
	}

	if strings.TrimSpace(req.RoleArn) == "" {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "RoleArn is required")
	}

	if strings.TrimSpace(req.S3UrlPrefix) == "" {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "S3UrlPrefix is required")
	}

	region := httputils.ExtractRegionFromRequest(c.Request(), h.DefaultRegion)

	job, backendErr := h.Backend.CreateExportJob(region, h.AccountID, appID, req)
	if backendErr != nil {
		if errors.Is(backendErr, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", backendErr.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", backendErr.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusCreated, toExportJobResponse(job))

	return nil
}

// handleCreateImportJob handles POST /v1/apps/{appId}/jobs/import.
func (h *Handler) handleCreateImportJob(c *echo.Context, appID string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "failed to read request body")
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	var req createImportJobRequest
	if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "invalid request body")
	}

	if strings.TrimSpace(req.RoleArn) == "" {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "RoleArn is required")
	}

	if strings.TrimSpace(req.S3Url) == "" {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "S3Url is required")
	}

	if strings.TrimSpace(req.Format) == "" {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "Format is required")
	}

	region := httputils.ExtractRegionFromRequest(c.Request(), h.DefaultRegion)

	job, backendErr := h.Backend.CreateImportJob(region, h.AccountID, appID, req)
	if backendErr != nil {
		if errors.Is(backendErr, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", backendErr.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", backendErr.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusCreated, toImportJobResponse(job))

	return nil
}

// handleGetExportJob handles GET /v1/apps/{appId}/jobs/export/{jobId}.
func (h *Handler) handleGetExportJob(c *echo.Context, appID, jobID string) error {
	job, err := h.Backend.GetExportJob(appID, jobID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toExportJobResponse(job))

	return nil
}

// handleGetExportJobs handles GET /v1/apps/{appId}/jobs/export.
func (h *Handler) handleGetExportJobs(c *echo.Context, appID string) error {
	jobs, err := h.Backend.GetExportJobs(appID)
	if err != nil {
		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	items := make([]exportJobResponse, 0, len(jobs))

	for _, j := range jobs {
		items = append(items, toExportJobResponse(j))
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, exportJobsListResponse{Item: items})

	return nil
}

// handleGetImportJob handles GET /v1/apps/{appId}/jobs/import/{jobId}.
func (h *Handler) handleGetImportJob(c *echo.Context, appID, jobID string) error {
	job, err := h.Backend.GetImportJob(appID, jobID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toImportJobResponse(job))

	return nil
}

// handleGetImportJobs handles GET /v1/apps/{appId}/jobs/import.
func (h *Handler) handleGetImportJobs(c *echo.Context, appID string) error {
	jobs, err := h.Backend.GetImportJobs(appID)
	if err != nil {
		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	items := make([]importJobResponse, 0, len(jobs))

	for _, j := range jobs {
		items = append(items, toImportJobResponse(j))
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, importJobsListResponse{Item: items})

	return nil
}

// ──────────────────────────────────────────────────
// Helper: writeNotFoundOrInternal
// ──────────────────────────────────────────────────

func toExportJobResponse(j *ExportJob) exportJobResponse {
	return exportJobResponse{
		ApplicationID: j.ApplicationID,
		ID:            j.ID,
		Definition: exportJobDefinition{
			RoleArn:     j.RoleArn,
			S3UrlPrefix: j.S3UrlPrefix,
		},
		JobStatus:    j.JobStatus,
		Type:         exportJobType,
		CreationDate: j.CreationDate,
	}
}

func toImportJobResponse(j *ImportJob) importJobResponse {
	return importJobResponse{
		ApplicationID: j.ApplicationID,
		ID:            j.ID,
		Definition: importJobDefinition{
			RoleArn:   j.RoleArn,
			S3Url:     j.S3Url,
			Format:    j.Format,
			SegmentID: j.SegmentID,
		},
		JobStatus:    j.JobStatus,
		Type:         importJobType,
		CreationDate: j.CreationDate,
	}
}
