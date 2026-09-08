package databrew

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func parseProfileJobOp(method, name string) string {
	switch method {
	case http.MethodPost:
		if name == "" {
			return opCreateProfileJob
		}
	case http.MethodPut:
		if name != "" {
			return opUpdateProfileJob
		}
	}

	return opUnknown
}

func parseRecipeJobOp(method, name string) string {
	switch method {
	case http.MethodPost:
		if name == "" {
			return opCreateRecipeJob
		}
	case http.MethodPut:
		if name != "" {
			return opUpdateRecipeJob
		}
	}

	return opUnknown
}

func parseJobOp(method, name, subOp string) string {
	switch {
	case subOp == "startJobRun" && method == http.MethodPost:

		return opStartJobRun
	case subOp == "jobRuns" && method == http.MethodGet:

		return opListJobRuns
	case strings.HasPrefix(subOp, "jobRun/") && method == http.MethodGet:
		return opDescribeJobRun
	case strings.HasPrefix(subOp, "jobRun/") && method == http.MethodPost:
		return opStopJobRun
	case method == http.MethodGet && name == "":

		return opListJobs
	case method == http.MethodGet && name != "":

		return opDescribeJob
	case method == http.MethodDelete && name != "":

		return opDeleteJob
	}

	return opUnknown
}

func (h *Handler) dispatchJob(
	ctx context.Context,
	action string,
	body []byte,
) ([]byte, bool, error) {
	switch action {
	case opCreateProfileJob:
		r, e := h.handleCreateProfileJob(ctx, body)

		return r, true, e
	case opCreateRecipeJob:
		r, e := h.handleCreateRecipeJob(ctx, body)

		return r, true, e
	case opDescribeJob:
		r, e := h.handleDescribeJob(ctx, body)

		return r, true, e
	case opListJobs:
		r, e := h.handleListJobs(ctx, body)

		return r, true, e
	case opUpdateProfileJob:
		r, e := h.handleUpdateProfileJob(ctx, body)

		return r, true, e
	case opUpdateRecipeJob:
		r, e := h.handleUpdateRecipeJob(ctx, body)

		return r, true, e
	case opDeleteJob:
		r, e := h.handleDeleteJob(ctx, body)

		return r, true, e
	case opStartJobRun:
		r, e := h.handleStartJobRun(ctx, body)

		return r, true, e
	case opListJobRuns:
		r, e := h.handleListJobRuns(ctx, body)

		return r, true, e
	case opDescribeJobRun:
		r, e := h.handleDescribeJobRun(ctx, body)

		return r, true, e
	case opStopJobRun:
		r, e := h.handleStopJobRun(ctx, body)

		return r, true, e
	}

	return nil, false, nil
}

func (h *Handler) handleCreateProfileJob(ctx context.Context, body []byte) ([]byte, error) {
	// CreateProfileJobInput's output field is a single "OutputLocation"
	// (S3Location), NOT the "Outputs" list CreateRecipeJobInput uses --
	// confirmed against aws-sdk-go-v2/service/databrew's serializer
	// (awsRestjson1_serializeOpDocumentCreateProfileJobInput writes the
	// "OutputLocation" JSON key). The Job entity itself still exposes this as
	// an Outputs list (see backend.Job.Outputs / DescribeJob), so it's
	// converted to a one-element Output slice for storage.
	var req struct {
		Tags                     map[string]string `json:"Tags"`
		OutputLocation           *S3Location       `json:"OutputLocation"`
		Configuration            map[string]any    `json:"Configuration"`
		JobSample                *JobSample        `json:"JobSample"`
		DatasetName              string            `json:"DatasetName"`
		Name                     string            `json:"Name"`
		RoleArn                  string            `json:"RoleArn"`
		EncryptionKeyArn         string            `json:"EncryptionKeyArn"`
		EncryptionMode           string            `json:"EncryptionMode"`
		LogSubscription          string            `json:"LogSubscription"`
		ValidationConfigurations []map[string]any  `json:"ValidationConfigurations"`
		MaxCapacity              int               `json:"MaxCapacity"`
		MaxRetries               int               `json:"MaxRetries"`
		Timeout                  int               `json:"Timeout"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}
	j, err := h.Backend.CreateJob(
		ctx,
		req.Name,
		"PROFILE",
		req.DatasetName,
		"",
		"",
		req.RoleArn,
		outputLocationToOutputs(req.OutputLocation),
		req.Tags,
		JobExtras{
			ProfileConfiguration:     req.Configuration,
			JobSample:                req.JobSample,
			ValidationConfigurations: req.ValidationConfigurations,
			EncryptionMode:           req.EncryptionMode,
			EncryptionKeyArn:         req.EncryptionKeyArn,
			LogSubscription:          req.LogSubscription,
			MaxCapacity:              req.MaxCapacity,
			MaxRetries:               req.MaxRetries,
			Timeout:                  req.Timeout,
		},
	)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{keyName: j.Name})
}

// outputLocationToOutputs converts a CreateProfileJobInput/UpdateProfileJobInput
// "OutputLocation" (a single S3Location) into the one-element Outputs slice
// the Job entity stores/returns, or nil when loc is unset.
func outputLocationToOutputs(loc *S3Location) []Output {
	if loc == nil {
		return nil
	}

	return []Output{{Location: *loc}}
}

func (h *Handler) handleCreateRecipeJob(ctx context.Context, body []byte) ([]byte, error) {
	var req struct {
		Tags               map[string]string   `json:"Tags"`
		RecipeReference    *RecipeRef          `json:"RecipeReference"`
		DataCatalogOutputs []DataCatalogOutput `json:"DataCatalogOutputs"`
		DatabaseOutputs    []DatabaseOutput    `json:"DatabaseOutputs"`
		Name               string              `json:"Name"`
		DatasetName        string              `json:"DatasetName"`
		ProjectName        string              `json:"ProjectName"`
		RoleArn            string              `json:"RoleArn"`
		EncryptionKeyArn   string              `json:"EncryptionKeyArn"`
		EncryptionMode     string              `json:"EncryptionMode"`
		LogSubscription    string              `json:"LogSubscription"`
		Outputs            []Output            `json:"Outputs"`
		MaxCapacity        int                 `json:"MaxCapacity"`
		MaxRetries         int                 `json:"MaxRetries"`
		Timeout            int                 `json:"Timeout"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}
	recipeName, recipeVersion := "", ""
	if req.RecipeReference != nil {
		recipeName = req.RecipeReference.Name
		recipeVersion = req.RecipeReference.RecipeVersion
	}
	j, err := h.Backend.CreateJob(
		ctx,
		req.Name,
		"RECIPE",
		req.DatasetName,
		req.ProjectName,
		recipeName,
		req.RoleArn,
		req.Outputs,
		req.Tags,
		JobExtras{
			RecipeVersion:      recipeVersion,
			EncryptionMode:     req.EncryptionMode,
			EncryptionKeyArn:   req.EncryptionKeyArn,
			LogSubscription:    req.LogSubscription,
			DataCatalogOutputs: req.DataCatalogOutputs,
			DatabaseOutputs:    req.DatabaseOutputs,
			MaxCapacity:        req.MaxCapacity,
			MaxRetries:         req.MaxRetries,
			Timeout:            req.Timeout,
		},
	)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{keyName: j.Name})
}

func (h *Handler) handleDescribeJob(ctx context.Context, body []byte) ([]byte, error) {
	var req struct {
		Name string `json:"Name"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}
	j, err := h.Backend.DescribeJob(ctx, req.Name)
	if err != nil {
		return nil, err
	}

	cp := *j
	cp.AccountID = ""

	return json.Marshal(cp)
}

func (h *Handler) handleListJobs(ctx context.Context, body []byte) ([]byte, error) {
	var req struct {
		MaxResults  string `json:"MaxResults"`
		NextToken   string `json:"NextToken"`
		DatasetName string `json:"DatasetName"`
		ProjectName string `json:"ProjectName"`
	}
	_ = json.Unmarshal(body, &req)
	maxResults, _ := strconv.Atoi(req.MaxResults)

	jobs, next := h.Backend.ListJobs(ctx, maxResults, req.NextToken, req.DatasetName, req.ProjectName)

	return json.Marshal(map[string]any{"Jobs": jobs, nextTokenKey: next})
}

// handleUpdateProfileJob handles UpdateProfileJob, whose wire field for the
// job's output destination is "OutputLocation" (a single S3Location), not
// "Outputs" -- see the outputLocationToOutputs doc comment.
func (h *Handler) handleUpdateProfileJob(ctx context.Context, body []byte) ([]byte, error) {
	var req struct {
		OutputLocation           *S3Location      `json:"OutputLocation"`
		Configuration            map[string]any   `json:"Configuration"`
		JobSample                *JobSample       `json:"JobSample"`
		Name                     string           `json:"Name"`
		RoleArn                  string           `json:"RoleArn"`
		EncryptionKeyArn         string           `json:"EncryptionKeyArn"`
		EncryptionMode           string           `json:"EncryptionMode"`
		LogSubscription          string           `json:"LogSubscription"`
		ValidationConfigurations []map[string]any `json:"ValidationConfigurations"`
		MaxCapacity              int              `json:"MaxCapacity"`
		MaxRetries               int              `json:"MaxRetries"`
		Timeout                  int              `json:"Timeout"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}
	if err := h.Backend.UpdateJob(
		ctx, req.Name, req.RoleArn, outputLocationToOutputs(req.OutputLocation),
		req.MaxCapacity, req.MaxRetries, req.Timeout,
		JobExtras{
			ProfileConfiguration:     req.Configuration,
			JobSample:                req.JobSample,
			ValidationConfigurations: req.ValidationConfigurations,
			EncryptionMode:           req.EncryptionMode,
			EncryptionKeyArn:         req.EncryptionKeyArn,
			LogSubscription:          req.LogSubscription,
		},
	); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{keyName: req.Name})
}

// handleUpdateRecipeJob handles UpdateRecipeJob, whose wire field for the
// job's output destinations is "Outputs" (a list), matching CreateRecipeJob.
func (h *Handler) handleUpdateRecipeJob(ctx context.Context, body []byte) ([]byte, error) {
	var req struct {
		Name               string              `json:"Name"`
		RoleArn            string              `json:"RoleArn"`
		EncryptionKeyArn   string              `json:"EncryptionKeyArn"`
		EncryptionMode     string              `json:"EncryptionMode"`
		LogSubscription    string              `json:"LogSubscription"`
		Outputs            []Output            `json:"Outputs"`
		DataCatalogOutputs []DataCatalogOutput `json:"DataCatalogOutputs"`
		DatabaseOutputs    []DatabaseOutput    `json:"DatabaseOutputs"`
		MaxCapacity        int                 `json:"MaxCapacity"`
		MaxRetries         int                 `json:"MaxRetries"`
		Timeout            int                 `json:"Timeout"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}
	if err := h.Backend.UpdateJob(
		ctx, req.Name, req.RoleArn, req.Outputs, req.MaxCapacity, req.MaxRetries, req.Timeout,
		JobExtras{
			EncryptionMode:     req.EncryptionMode,
			EncryptionKeyArn:   req.EncryptionKeyArn,
			LogSubscription:    req.LogSubscription,
			DataCatalogOutputs: req.DataCatalogOutputs,
			DatabaseOutputs:    req.DatabaseOutputs,
		},
	); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{keyName: req.Name})
}

func (h *Handler) handleDeleteJob(ctx context.Context, body []byte) ([]byte, error) {
	var req struct {
		Name string `json:"Name"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}
	if err := h.Backend.DeleteJob(ctx, req.Name); err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{keyName: req.Name})
}

func (h *Handler) handleStartJobRun(ctx context.Context, body []byte) ([]byte, error) {
	var req struct {
		Name string `json:"Name"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}
	run, err := h.Backend.StartJobRun(ctx, req.Name)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"RunId": run.RunID})
}

func (h *Handler) handleListJobRuns(ctx context.Context, body []byte) ([]byte, error) {
	var req struct {
		Name       string `json:"Name"`
		MaxResults string `json:"MaxResults"`
		NextToken  string `json:"NextToken"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}
	maxResults, _ := strconv.Atoi(req.MaxResults)

	runs, next, err := h.Backend.ListJobRuns(ctx, req.Name, maxResults, req.NextToken)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{"JobRuns": runs, nextTokenKey: next})
}

func (h *Handler) handleDescribeJobRun(ctx context.Context, body []byte) ([]byte, error) {
	var req struct {
		Name  string `json:"Name"`
		RunID string `json:"RunId"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}
	run, err := h.Backend.DescribeJobRun(ctx, req.Name, req.RunID)
	if err != nil {
		return nil, err
	}

	return json.Marshal(run)
}

func (h *Handler) handleStopJobRun(ctx context.Context, body []byte) ([]byte, error) {
	var req struct {
		Name  string `json:"Name"`
		RunID string `json:"RunId"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}
	run, err := h.Backend.StopJobRun(ctx, req.Name, req.RunID)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{"RunId": run.RunID})
}
