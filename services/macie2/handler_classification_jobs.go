package macie2

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

func parseJobPath(method string, parts []string) (string, string) {
	switch len(parts) {
	case depthRoot: // /jobs
		if method == http.MethodPost {
			return opCreateClassificationJob, ""
		}
	case depthResource: // /jobs/{jobId|list}
		switch parts[1] {
		case "list": //nolint:goconst // existing issue.
			if method == http.MethodPost {
				return opListClassificationJobs, ""
			}
		default:
			switch method {
			case http.MethodGet:
				return opDescribeClassificationJob, parts[1]
			case http.MethodPatch:
				return opUpdateClassificationJob, parts[1]
			}
		}
	}

	return opUnknown, ""
}

func parseClassExportCfgPath(method string, _ []string) (string, string) {
	switch method {
	case http.MethodGet:
		return opGetClassificationExportConfiguration, ""
	case http.MethodPut:
		return opPutClassificationExportConfiguration, ""
	}

	return opUnknown, ""
}

func parseClassScopesPath(method string, parts []string) (string, string) {
	switch len(parts) {
	case depthRoot: // /classification-scopes
		if method == http.MethodGet {
			return opListClassificationScopes, ""
		}
	case depthResource: // /classification-scopes/{id}
		switch method {
		case http.MethodGet:
			return opGetClassificationScope, parts[1]
		case http.MethodPatch:
			return opUpdateClassificationScope, parts[1]
		}
	}

	return opUnknown, ""
}

func (h *Handler) dispatchClassificationJobOps(op, path string, body []byte) (any, int, bool, error) {
	switch op {
	case opCreateClassificationJob:
		result, code, err := h.handleCreateClassificationJob(body)

		return result, code, true, err

	case opDescribeClassificationJob:
		id := extractID(path, pathJobs)
		result, code, err := h.handleDescribeClassificationJob(id)

		return result, code, true, err

	case opListClassificationJobs:
		result, code, err := h.handleListClassificationJobs(body)

		return result, code, true, err

	case opUpdateClassificationJob:
		id := extractID(path, pathJobs)
		code, err := h.handleUpdateClassificationJob(id, body)

		return nil, code, true, err
	}

	return nil, 0, false, nil
}

func (h *Handler) dispatchScopeOps(op, path string, body []byte) (any, int, bool, error) {
	switch op {
	case opGetClassificationScope:
		id := extractID(path, pathClassScopes)
		result, code, err := h.handleGetClassificationScope(id)

		return result, code, true, err

	case opListClassificationScopes:
		result, code, err := h.handleListClassificationScopes()

		return result, code, true, err

	case opUpdateClassificationScope:
		id := extractID(path, pathClassScopes)
		code, err := h.handleUpdateClassificationScope(id, body)

		return nil, code, true, err
	}

	return nil, 0, false, nil
}

func (h *Handler) dispatchClassificationExportConfigOps(op string, body []byte) (any, int, bool, error) {
	switch op {
	case opGetClassificationExportConfiguration:
		result, code, err := h.handleGetClassificationExportConfiguration()

		return result, code, true, err

	case opPutClassificationExportConfiguration:
		code, err := h.handlePutClassificationExportConfiguration(body)

		return nil, code, true, err
	}

	return nil, 0, false, nil
}

func (h *Handler) handleCreateClassificationJob(body []byte) (any, int, error) {
	var req struct {
		Tags                          map[string]string `json:"tags"`
		S3JobDefinition               map[string]any    `json:"s3JobDefinition"`
		ScheduleFrequency             map[string]any    `json:"scheduleFrequency"`
		ClientToken                   string            `json:"clientToken"`
		Description                   string            `json:"description"`
		JobType                       string            `json:"jobType"`
		Name                          string            `json:"name"`
		ManagedDataIdentifierSelector string            `json:"managedDataIdentifierSelector"`
		AllowListIDs                  []string          `json:"allowListIds"`
		CustomDataIdentifierIDs       []string          `json:"customDataIdentifierIds"`
		ManagedDataIdentifierIDs      []string          `json:"managedDataIdentifierIds"`
		SamplingPercentage            int32             `json:"samplingPercentage"`
		InitialRun                    bool              `json:"initialRun"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, http.StatusBadRequest, ErrValidation
	}

	if req.Name == "" || req.JobType == "" {
		return nil, http.StatusBadRequest, ErrValidation
	}

	id, jobArn, err := h.Backend.CreateClassificationJob(
		req.Name, req.Description, req.JobType, req.ClientToken,
		req.S3JobDefinition, req.ScheduleFrequency,
		req.AllowListIDs, req.CustomDataIdentifierIDs, req.ManagedDataIdentifierIDs,
		req.ManagedDataIdentifierSelector,
		req.Tags, req.SamplingPercentage, req.InitialRun,
	)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	return map[string]string{"jobArn": jobArn, "jobId": id, keyJobStatus: jobStatusRunning}, http.StatusOK, nil
}

func (h *Handler) handleDescribeClassificationJob(jobID string) (any, int, error) {
	job, err := h.Backend.DescribeClassificationJob(jobID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return nil, http.StatusNotFound, err
		}

		return nil, http.StatusInternalServerError, err
	}

	return job, http.StatusOK, nil
}

func (h *Handler) handleListClassificationJobs(body []byte) (any, int, error) {
	var req struct {
		FilterCriteria map[string]any `json:"filterCriteria"`
		SortCriteria   *struct {
			AttributeName string `json:"attributeName"`
			OrderBy       string `json:"orderBy"`
		} `json:"sortCriteria"`
		NextToken  string `json:"nextToken"`
		MaxResults int    `json:"maxResults"`
	}

	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, http.StatusBadRequest, ErrValidation
		}
	}

	var sortBy *ListJobsSortCriteria
	if req.SortCriteria != nil {
		sortBy = &ListJobsSortCriteria{
			AttributeName: req.SortCriteria.AttributeName, OrderBy: req.SortCriteria.OrderBy,
		}
	}

	jobs, nextToken, err := h.Backend.ListClassificationJobs(req.FilterCriteria, sortBy, req.MaxResults, req.NextToken)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	resp := map[string]any{keyItems: jobs}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return resp, http.StatusOK, nil
}

func (h *Handler) handleUpdateClassificationJob(jobID string, body []byte) (int, error) {
	var req struct {
		JobStatus string `json:"jobStatus"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return http.StatusBadRequest, ErrValidation
	}

	if err := h.Backend.UpdateClassificationJob(jobID, req.JobStatus); err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return http.StatusNotFound, err
		}

		if errors.Is(err, awserr.ErrConflict) {
			return http.StatusConflict, err
		}

		if errors.Is(err, awserr.ErrInvalidParameter) {
			return http.StatusBadRequest, err
		}

		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}

func (h *Handler) handleGetClassificationExportConfiguration() (any, int, error) {
	cfg, err := h.Backend.GetClassificationExportConfiguration()
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	return map[string]any{keyConfiguration: cfg}, http.StatusOK, nil
}

func (h *Handler) handlePutClassificationExportConfiguration(body []byte) (int, error) {
	var req struct {
		Configuration *ClassificationExportConfig `json:"configuration"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return http.StatusBadRequest, ErrValidation
	}

	if err := h.Backend.PutClassificationExportConfiguration(req.Configuration); err != nil {
		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}

func (h *Handler) handleGetClassificationScope(scopeID string) (any, int, error) {
	scope, err := h.Backend.GetClassificationScope(scopeID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return nil, http.StatusNotFound, err
		}

		return nil, http.StatusInternalServerError, err
	}

	return scope, http.StatusOK, nil
}

func (h *Handler) handleListClassificationScopes() (any, int, error) {
	scopes, err := h.Backend.ListClassificationScopes()
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	return map[string]any{"classificationScopes": scopes}, http.StatusOK, nil
}

func (h *Handler) handleUpdateClassificationScope(scopeID string, body []byte) (int, error) {
	var req struct {
		S3 *ClassificationScopeS3Update `json:"s3"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return http.StatusBadRequest, ErrValidation
	}

	if err := h.Backend.UpdateClassificationScope(scopeID, req.S3); err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return http.StatusNotFound, err
		}

		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}
