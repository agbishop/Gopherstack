package fis

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"
)

// ----------------------------------------
// Experiment handlers
// ----------------------------------------

func (h *Handler) handleStartExperiment(ctx context.Context, c *echo.Context, body []byte) error {
	var input startExperimentRequest
	if err := json.Unmarshal(body, &input); err != nil {
		return h.writeError(c, http.StatusBadRequest, "invalid request body: "+err.Error(), "")
	}

	exp, err := h.Backend.StartExperiment(ctx, &input, h.AccountID, h.DefaultRegion)
	if err != nil {
		return h.writeBackendError(c, err, input.ExperimentTemplateID)
	}

	return c.JSON(http.StatusCreated, experimentResponseDTO{
		Experiment: toExperimentDTO(exp),
	})
}

func (h *Handler) handleGetExperiment(c *echo.Context, id string) error {
	exp, err := h.Backend.GetExperiment(id)
	if err != nil {
		return h.writeBackendError(c, err, id)
	}

	return c.JSON(http.StatusOK, experimentResponseDTO{
		Experiment: toExperimentDTO(exp),
	})
}

func (h *Handler) handleStopExperiment(c *echo.Context, id string) error {
	exp, err := h.Backend.StopExperiment(id)
	if err != nil {
		return h.writeBackendError(c, err, id)
	}

	return c.JSON(http.StatusOK, experimentResponseDTO{
		Experiment: toExperimentDTO(exp),
	})
}

func (h *Handler) handleListExperiments(c *echo.Context) error {
	experiments, err := h.Backend.ListExperiments()
	if err != nil {
		return h.writeError(c, http.StatusInternalServerError, err.Error(), "")
	}

	q := c.Request().URL.Query()

	// Filter by experimentTemplateId.
	if tplFilter := q.Get("experimentTemplateId"); tplFilter != "" {
		filtered := experiments[:0]

		for _, e := range experiments {
			if e.ExperimentTemplateID == tplFilter {
				filtered = append(filtered, e)
			}
		}

		experiments = filtered
	}

	// Filter by status.
	if statusFilter := q.Get("status"); statusFilter != "" {
		filtered := experiments[:0]

		for _, e := range experiments {
			if e.Status.Status == statusFilter {
				filtered = append(filtered, e)
			}
		}

		experiments = filtered
	}

	// Apply cursor-based pagination.
	ids := make([]string, len(experiments))
	for i, e := range experiments {
		ids[i] = e.ID
	}

	page, nextTok := paginatePage(experiments, ids, q)
	dtos := make([]experimentSummaryDTO, len(page))

	for i, e := range page {
		dtos[i] = toExperimentSummaryDTO(e)
	}

	return c.JSON(http.StatusOK, listExperimentsResponseDTO{
		Experiments: dtos,
		NextToken:   nextTok,
	})
}

// ----------------------------------------
// Resolved targets handlers
// ----------------------------------------

func (h *Handler) handleListExperimentResolvedTargets(c *echo.Context, id string) error {
	resolved, err := h.Backend.ListExperimentResolvedTargets(id)
	if err != nil {
		return h.writeBackendError(c, err, id)
	}

	names := make([]string, len(resolved))
	for i, rt := range resolved {
		names[i] = rt.TargetName
	}

	page, nextTok := paginatePage(resolved, names, c.Request().URL.Query())
	dtos := make([]resolvedTargetDTO, len(page))

	for i, rt := range page {
		dtos[i] = resolvedTargetDTO{ResourceType: rt.ResourceType, TargetName: rt.TargetName}
	}

	return c.JSON(http.StatusOK, listExperimentResolvedTargetsResponseDTO{
		ResolvedTargets: dtos,
		NextToken:       nextTok,
	})
}

// ----------------------------------------
// DTO conversion helpers
// ----------------------------------------

// experimentTargetDTOs converts a running experiment's resolved targets to wire DTOs.
func experimentTargetDTOs(targets map[string]ExperimentTarget) map[string]experimentTargetDTO {
	dtos := make(map[string]experimentTargetDTO, len(targets))

	for name, t := range targets {
		filters := make([]experimentTemplateTargetFilterDTO, len(t.Filters))
		for i, f := range t.Filters {
			filters[i] = experimentTemplateTargetFilterDTO(f)
		}

		dtos[name] = experimentTargetDTO{
			ResourceType:  t.ResourceType,
			SelectionMode: t.SelectionMode,
			ResourceArns:  t.ResourceArns,
			ResourceTags:  t.ResourceTags,
			Filters:       filters,
			Parameters:    t.Parameters,
		}
	}

	return dtos
}

// experimentActionDTOs converts a running experiment's actions to wire DTOs.
func experimentActionDTOs(actions map[string]ExperimentAction) map[string]experimentActionDTO {
	dtos := make(map[string]experimentActionDTO, len(actions))

	for name, a := range actions {
		dtos[name] = experimentActionDTO{
			ActionID:    a.ActionID,
			Description: a.Description,
			Parameters:  a.Parameters,
			Targets:     a.Targets,
			StartAfter:  a.StartAfter,
			Status: &experimentActionStatusDTO{
				Status: a.Status.Status,
				Reason: a.Status.Reason,
			},
			State: &experimentActionStatusDTO{
				Status: a.Status.Status,
				Reason: a.Status.Reason,
			},
			StartTime: toUnixPtr(a.StartTime),
			EndTime:   toUnixPtr(a.EndTime),
		}
	}

	return dtos
}

// experimentLogConfigDTO converts a running experiment's log configuration to its wire DTO.
func experimentLogConfigDTO(logConfig *ExperimentLogConfiguration) *experimentLogConfigurationDTO {
	if logConfig == nil {
		return nil
	}

	lc := &experimentLogConfigurationDTO{LogSchemaVersion: logConfig.LogSchemaVersion}

	if logConfig.CloudWatchLogsConfiguration != nil {
		lc.CloudWatchLogsConfiguration = &experimentCloudWatchLogsConfigurationDTO{
			LogGroupArn: logConfig.CloudWatchLogsConfiguration.LogGroupArn,
		}
	}

	if logConfig.S3Configuration != nil {
		lc.S3Configuration = &experimentS3ConfigurationDTO{
			BucketName: logConfig.S3Configuration.BucketName,
			Prefix:     logConfig.S3Configuration.Prefix,
		}
	}

	return lc
}

func toExperimentSummaryDTO(exp *Experiment) experimentSummaryDTO {
	statusDTO := experimentStatusDTO{
		Status: exp.Status.Status,
		Reason: exp.Status.Reason,
	}

	if exp.Status.Error != nil {
		statusDTO.Error = &experimentStatusErrorDTO{
			Code:      exp.Status.Error.Code,
			Location:  exp.Status.Error.Location,
			AccountID: exp.Status.Error.AccountID,
		}
	}

	dto := experimentSummaryDTO{
		ID:                   exp.ID,
		Arn:                  exp.Arn,
		ExperimentTemplateID: exp.ExperimentTemplateID,
		State:                statusDTO,
		Tags:                 exp.Tags,
		CreationTime:         toUnix(exp.CreationTime),
	}

	if exp.ExperimentOptions != nil {
		dto.ExperimentOptions = &experimentExperimentOptionsDTO{
			AccountTargeting:          exp.ExperimentOptions.AccountTargeting,
			EmptyTargetResolutionMode: exp.ExperimentOptions.EmptyTargetResolutionMode,
			ActionsMode:               exp.ExperimentOptions.ActionsMode,
		}
	}

	return dto
}

func toExperimentDTO(exp *Experiment) experimentDTO {
	targets := experimentTargetDTOs(exp.Targets)
	actions := experimentActionDTOs(exp.Actions)

	stopConditions := make([]experimentStopConditionDTO, len(exp.StopConditions))
	for i, sc := range exp.StopConditions {
		stopConditions[i] = experimentStopConditionDTO(sc)
	}

	statusDTO := experimentStatusDTO{
		Status: exp.Status.Status,
		Reason: exp.Status.Reason,
	}

	if exp.Status.Error != nil {
		statusDTO.Error = &experimentStatusErrorDTO{
			Code:      exp.Status.Error.Code,
			Location:  exp.Status.Error.Location,
			AccountID: exp.Status.Error.AccountID,
		}
	}

	dto := experimentDTO{
		ID:                               exp.ID,
		Arn:                              exp.Arn,
		ExperimentTemplateID:             exp.ExperimentTemplateID,
		RoleArn:                          exp.RoleArn,
		Status:                           statusDTO,
		State:                            statusDTO,
		Targets:                          targets,
		Actions:                          actions,
		StopConditions:                   stopConditions,
		Tags:                             exp.Tags,
		CreationTime:                     toUnix(exp.CreationTime),
		StartTime:                        toUnix(exp.StartTime),
		EndTime:                          toUnixPtr(exp.EndTime),
		TargetAccountConfigurationsCount: exp.TargetAccountConfigurationsCount,

		LogConfiguration: experimentLogConfigDTO(exp.LogConfiguration)}

	if exp.ExperimentOptions != nil {
		dto.ExperimentOptions = &experimentExperimentOptionsDTO{
			AccountTargeting:          exp.ExperimentOptions.AccountTargeting,
			EmptyTargetResolutionMode: exp.ExperimentOptions.EmptyTargetResolutionMode,
			ActionsMode:               exp.ExperimentOptions.ActionsMode,
		}
	}

	if exp.ExperimentReportConfiguration != nil {
		dto.ExperimentReportConfiguration = toExperimentReportConfigDTO(exp.ExperimentReportConfiguration)
	}

	if exp.ExperimentReport != nil {
		dto.ExperimentReport = toExperimentReportDTO(exp.ExperimentReport)
	}

	return dto
}

// toExperimentReportConfigDTO converts a running experiment's report
// configuration domain type into its wire DTO.
func toExperimentReportConfigDTO(cfg *ExperimentReportConfiguration) *experimentReportConfigurationDTO {
	dto := &experimentReportConfigurationDTO{
		PreExperimentDuration:  cfg.PreExperimentDuration,
		PostExperimentDuration: cfg.PostExperimentDuration,
	}

	if cfg.DataSources != nil {
		dashboards := make(
			[]experimentReportConfigurationCloudWatchDashboardDTO,
			len(cfg.DataSources.CloudWatchDashboards),
		)
		for i, d := range cfg.DataSources.CloudWatchDashboards {
			dashboards[i] = experimentReportConfigurationCloudWatchDashboardDTO(d)
		}

		dto.DataSources = &experimentReportConfigurationDataSourcesDTO{CloudWatchDashboards: dashboards}
	}

	if cfg.Outputs != nil && cfg.Outputs.S3Configuration != nil {
		dto.Outputs = &experimentReportConfigurationOutputsDTO{
			S3Configuration: &experimentReportConfigurationOutputsS3ConfigurationDTO{
				BucketName: cfg.Outputs.S3Configuration.BucketName,
				Prefix:     cfg.Outputs.S3Configuration.Prefix,
			},
		}
	}

	return dto
}

// toExperimentReportDTO converts a running experiment's generated report domain
// type into its wire DTO.
func toExperimentReportDTO(rep *ExperimentReport) *experimentReportDTO {
	dto := &experimentReportDTO{}

	if len(rep.S3Reports) > 0 {
		dto.S3Reports = make([]experimentReportS3ReportDTO, len(rep.S3Reports))
		for i, r := range rep.S3Reports {
			dto.S3Reports[i] = experimentReportS3ReportDTO(r)
		}
	}

	if rep.State != nil {
		dto.State = &experimentReportStateDTO{Status: rep.State.Status, Reason: rep.State.Reason}

		if rep.State.Error != nil {
			dto.State.Error = &experimentReportErrorDTO{Code: rep.State.Error.Code}
		}
	}

	return dto
}
