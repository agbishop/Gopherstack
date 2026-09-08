package bedrock

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
)

func extractCustomizationJobOperation(path, method string) (string, bool) {
	isSubPath := strings.HasPrefix(path, modelCustomizationJobsPrefix+"/")
	isStop := isSubPath && strings.HasSuffix(path, "/stop")

	switch {
	case path == modelCustomizationJobsPrefix && method == http.MethodPost:
		return "CreateModelCustomizationJob", true
	case path == modelCustomizationJobsPrefix && method == http.MethodGet:
		return "ListModelCustomizationJobs", true
	case isSubPath && method == http.MethodGet && !isStop:
		return "GetModelCustomizationJob", true
	case isStop && method == http.MethodPost:
		return "StopModelCustomizationJob", true
	default:
		return "", false
	}
}

func (h *Handler) routeCustomizationJob(
	c *echo.Context,
	path, method string,
	body []byte,
) (bool, error) {
	isSubPath := strings.HasPrefix(path, modelCustomizationJobsPrefix+"/")
	isStop := isSubPath && strings.HasSuffix(path, "/stop")

	switch {
	case path == modelCustomizationJobsPrefix && method == http.MethodPost:
		return true, h.handleCreateModelCustomizationJob(c, body)
	case path == modelCustomizationJobsPrefix && method == http.MethodGet:
		return true, h.handleListModelCustomizationJobs(c)
	case isSubPath && method == http.MethodGet && !isStop:
		id := decodePath(strings.TrimPrefix(path, modelCustomizationJobsPrefix+"/"))

		return true, h.handleGetModelCustomizationJob(c, id)
	case isStop && method == http.MethodPost:
		rest := strings.TrimPrefix(path, modelCustomizationJobsPrefix+"/")
		id := decodePath(strings.TrimSuffix(rest, "/stop"))

		return true, h.handleStopModelCustomizationJob(c, id)
	default:
		return false, nil
	}
}

// outputDataConfigInput mirrors bedrock@v1.66.4 types.OutputDataConfig
// (serializers.go: awsRestjson1_serializeDocumentOutputDataConfig emits
// {"s3Uri": ...}).
type outputDataConfigInput struct {
	S3Uri string `json:"s3Uri"`
}

// invocationLogSourceInput mirrors bedrock@v1.66.4
// types.InvocationLogSourceMemberS3Uri, the union's only member
// (serializers.go: awsRestjson1_serializeDocumentInvocationLogSource emits
// {"s3Uri": ...} for it).
type invocationLogSourceInput struct {
	S3Uri string `json:"s3Uri"`
}

// invocationLogsConfigInput mirrors bedrock@v1.66.4 types.InvocationLogsConfig
// (serializers.go: awsRestjson1_serializeDocumentInvocationLogsConfig).
// RequestMetadataFilters is intentionally not decoded here -- see
// TrainingDataConfig's doc comment in models.go for why.
type invocationLogsConfigInput struct {
	InvocationLogSource *invocationLogSourceInput `json:"invocationLogSource,omitempty"`
	UsePromptResponse   bool                      `json:"usePromptResponse,omitempty"`
}

// trainingDataConfigInput mirrors bedrock@v1.66.4 types.TrainingDataConfig
// (serializers.go: awsRestjson1_serializeDocumentTrainingDataConfig).
type trainingDataConfigInput struct {
	InvocationLogsConfig *invocationLogsConfigInput `json:"invocationLogsConfig,omitempty"`
	S3Uri                string                     `json:"s3Uri,omitempty"`
}

// validatorInput mirrors bedrock@v1.66.4 types.Validator (serializers.go:13263).
type validatorInput struct {
	S3Uri string `json:"s3Uri"`
}

// validationDataConfigInput mirrors bedrock@v1.66.4 types.ValidationDataConfig
// (serializers.go:13249).
type validationDataConfigInput struct {
	Validators []validatorInput `json:"validators,omitempty"`
}

func (v *validationDataConfigInput) s3Uris() []string {
	if v == nil {
		return nil
	}

	uris := make([]string, 0, len(v.Validators))
	for _, validator := range v.Validators {
		uris = append(uris, validator.S3Uri)
	}

	return uris
}

func (t *trainingDataConfigInput) toModel() TrainingDataConfig {
	if t == nil {
		return TrainingDataConfig{}
	}

	cfg := TrainingDataConfig{S3Uri: t.S3Uri}

	if t.InvocationLogsConfig != nil {
		cfg.UsePromptResponse = t.InvocationLogsConfig.UsePromptResponse

		if t.InvocationLogsConfig.InvocationLogSource != nil {
			cfg.InvocationLogSourceS3Uri = t.InvocationLogsConfig.InvocationLogSource.S3Uri
		}
	}

	return cfg
}

type createModelCustomizationJobInput struct {
	JobName             string                 `json:"jobName"`
	CustomModelName     string                 `json:"customModelName"`
	BaseModelIdentifier string                 `json:"baseModelIdentifier"`
	CustomizationType   string                 `json:"customizationType,omitempty"`
	RoleArn             string                 `json:"roleArn"`
	OutputDataConfig    *outputDataConfigInput `json:"outputDataConfig"`
	// TrainingDataConfig must be a pointer so an absent object (vs. one
	// present but empty -- valid, since neither of its own leaves is
	// required) can be distinguished for the "trainingDataConfig is
	// required" check below.
	TrainingDataConfig *trainingDataConfigInput `json:"trainingDataConfig"`
	// ValidationDataConfig is optional on input but GetModelCustomizationJobOutput's
	// own ValidationDataConfig is required regardless (bedrock@v1.66.4
	// api_op_GetModelCustomizationJob.go:85) -- see modelCustomizationJobOutput.
	ValidationDataConfig *validationDataConfigInput `json:"validationDataConfig,omitempty"`
	// JobTags, not Tags: real CreateModelCustomizationJobInput carries the
	// job's own tags as JobTags (wire key "jobTags"), separate from
	// CustomModelTags (wire key "customModelTags") on the resulting output
	// model, which gopherstack does not track as an independently taggable
	// resource (bedrock@v1.66.4 serializers.go:
	// awsRestjson1_serializeOpDocumentCreateModelCustomizationJobInput).
	Tags []Tag `json:"jobTags,omitempty"`
}

type createModelCustomizationJobOutput struct {
	JobArn string `json:"jobArn"`
}

func (h *Handler) handleCreateModelCustomizationJob(c *echo.Context, body []byte) error {
	in, err := parseBody[createModelCustomizationJobInput](body)
	if err != nil {
		return c.JSON(
			http.StatusBadRequest,
			errorResponse("ValidationException", "invalid request body"),
		)
	}

	if in.TrainingDataConfig == nil {
		return c.JSON(
			http.StatusBadRequest,
			errorResponse("ValidationException", "trainingDataConfig is required"),
		)
	}

	var outputDataConfig OutputDataConfig
	if in.OutputDataConfig != nil {
		outputDataConfig = OutputDataConfig{S3Uri: in.OutputDataConfig.S3Uri}
	}

	job, opErr := h.Backend.CreateModelCustomizationJob(
		in.JobName, in.CustomModelName, in.BaseModelIdentifier, in.CustomizationType, in.RoleArn,
		outputDataConfig, in.TrainingDataConfig.toModel(), in.ValidationDataConfig.s3Uris(), in.Tags,
	)
	if opErr != nil {
		return h.writeError(c, opErr)
	}

	return c.JSON(http.StatusCreated, createModelCustomizationJobOutput{JobArn: job.JobArn})
}

// modelCustomizationJobOutput is GetModelCustomizationJob's response shape
// (bedrock@v1.66.4 GetModelCustomizationJobResponse via botocore
// service-2.json: outputModelArn/outputModelName, not customModelArn/Name --
// see modelCustomizationJobSummaryOutput for the distinct ListModelCustomizationJobs
// shape).
// outputDataConfigOutput/trainingDataConfigOutput mirror the input-side wire
// shapes above; kept distinct so the input side's *pointer, required-object-
// detecting* shape doesn't leak into what's always a fully-populated output.
type outputDataConfigOutput struct {
	S3Uri string `json:"s3Uri"`
}

type validatorOutput struct {
	S3Uri string `json:"s3Uri"`
}

// validationDataConfigOutput is required on GetModelCustomizationJobOutput
// (bedrock@v1.66.4 api_op_GetModelCustomizationJob.go:85) even when no
// validators were supplied on Create -- emitted present-and-empty, never
// omitted, matching this service's existing convention for required objects
// with nothing real to report.
type validationDataConfigOutput struct {
	Validators []validatorOutput `json:"validators"`
}

func validationDataConfigToOutput(uris []string) validationDataConfigOutput {
	validators := make([]validatorOutput, 0, len(uris))
	for _, uri := range uris {
		validators = append(validators, validatorOutput{S3Uri: uri})
	}

	return validationDataConfigOutput{Validators: validators}
}

type trainingDataConfigOutput struct {
	InvocationLogsConfig *invocationLogsConfigOutput `json:"invocationLogsConfig,omitempty"`
	S3Uri                string                      `json:"s3Uri,omitempty"`
}

type invocationLogsConfigOutput struct {
	InvocationLogSource *invocationLogSourceInput `json:"invocationLogSource,omitempty"`
	UsePromptResponse   bool                      `json:"usePromptResponse,omitempty"`
}

func trainingDataConfigToOutput(t TrainingDataConfig) trainingDataConfigOutput {
	out := trainingDataConfigOutput{S3Uri: t.S3Uri}

	if t.InvocationLogSourceS3Uri != "" || t.UsePromptResponse {
		out.InvocationLogsConfig = &invocationLogsConfigOutput{
			UsePromptResponse: t.UsePromptResponse,
		}

		if t.InvocationLogSourceS3Uri != "" {
			out.InvocationLogsConfig.InvocationLogSource = &invocationLogSourceInput{S3Uri: t.InvocationLogSourceS3Uri}
		}
	}

	return out
}

type modelCustomizationJobOutput struct {
	CreationTime         string                     `json:"creationTime"`
	LastModifiedTime     string                     `json:"lastModifiedTime"`
	JobArn               string                     `json:"jobArn"`
	JobName              string                     `json:"jobName"`
	BaseModelArn         string                     `json:"baseModelArn"`
	OutputModelArn       string                     `json:"outputModelArn"`
	OutputModelName      string                     `json:"outputModelName"`
	Status               string                     `json:"status"`
	CustomizationType    string                     `json:"customizationType,omitempty"`
	RoleArn              string                     `json:"roleArn"`
	OutputDataConfig     outputDataConfigOutput     `json:"outputDataConfig"`
	TrainingDataConfig   trainingDataConfigOutput   `json:"trainingDataConfig"`
	ValidationDataConfig validationDataConfigOutput `json:"validationDataConfig"`
	Tags                 []Tag                      `json:"tags,omitempty"`
}

func customizationJobToOutput(j *ModelCustomizationJob) modelCustomizationJobOutput {
	return modelCustomizationJobOutput{
		JobArn:               j.JobArn,
		JobName:              j.JobName,
		BaseModelArn:         j.BaseModelArn,
		OutputModelArn:       j.OutputModelArn,
		OutputModelName:      j.CustomModelName,
		Status:               j.Status,
		CustomizationType:    j.CustomizationType,
		RoleArn:              j.RoleArn,
		OutputDataConfig:     outputDataConfigOutput{S3Uri: j.OutputDataConfig.S3Uri},
		TrainingDataConfig:   trainingDataConfigToOutput(j.TrainingDataConfig),
		ValidationDataConfig: validationDataConfigToOutput(j.ValidatorS3Uris),
		CreationTime:         j.CreationTime.Format(time.RFC3339),
		LastModifiedTime:     j.LastModifiedTime.Format(time.RFC3339),
		Tags:                 j.Tags,
	}
}

// modelCustomizationJobSummaryOutput is ListModelCustomizationJobs' per-item
// shape (bedrock@v1.66.4 ModelCustomizationJobSummary via botocore
// service-2.json): customModelArn/customModelName, distinct from Get's
// outputModelArn/outputModelName.
type modelCustomizationJobSummaryOutput struct {
	CreationTime      string `json:"creationTime"`
	LastModifiedTime  string `json:"lastModifiedTime"`
	JobArn            string `json:"jobArn"`
	JobName           string `json:"jobName"`
	BaseModelArn      string `json:"baseModelArn"`
	CustomModelArn    string `json:"customModelArn,omitempty"`
	CustomModelName   string `json:"customModelName,omitempty"`
	Status            string `json:"status"`
	CustomizationType string `json:"customizationType,omitempty"`
}

func customizationJobToSummaryOutput(j *ModelCustomizationJob) modelCustomizationJobSummaryOutput {
	return modelCustomizationJobSummaryOutput{
		JobArn:            j.JobArn,
		JobName:           j.JobName,
		BaseModelArn:      j.BaseModelArn,
		CustomModelArn:    j.OutputModelArn,
		CustomModelName:   j.CustomModelName,
		Status:            j.Status,
		CustomizationType: j.CustomizationType,
		CreationTime:      j.CreationTime.Format(time.RFC3339),
		LastModifiedTime:  j.LastModifiedTime.Format(time.RFC3339),
	}
}

func (h *Handler) handleGetModelCustomizationJob(c *echo.Context, id string) error {
	job, err := h.Backend.GetModelCustomizationJob(id)
	if err != nil {
		return h.writeError(c, err)
	}

	return c.JSON(http.StatusOK, customizationJobToOutput(job))
}

type listModelCustomizationJobsOutput struct {
	NextToken                      string                               `json:"nextToken,omitempty"`
	ModelCustomizationJobSummaries []modelCustomizationJobSummaryOutput `json:"modelCustomizationJobSummaries"`
}

// parseListModelCustomizationJobsQuery builds the backend filter/sort/pagination
// input from the real ListModelCustomizationJobs query-string bindings
// (aws-sdk-go-v2 serializers.go:6989-7027): statusEquals, nameContains,
// creationTimeAfter/Before, sortBy, sortOrder, nextToken.
//
//nolint:dupl // mirrors sibling List*Query parsers over a distinct filter set.
func parseListModelCustomizationJobsQuery(c *echo.Context) *ListModelCustomizationJobsInput {
	q := c.Request().URL.Query()

	maxResults, _ := strconv.ParseInt(q.Get("maxResults"), 10, 32)

	in := &ListModelCustomizationJobsInput{
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

func (h *Handler) handleListModelCustomizationJobs(c *echo.Context) error {
	jobs, outToken := h.Backend.ListModelCustomizationJobs(parseListModelCustomizationJobsQuery(c))
	summaries := make([]modelCustomizationJobSummaryOutput, 0, len(jobs))

	for _, j := range jobs {
		summaries = append(summaries, customizationJobToSummaryOutput(j))
	}

	return c.JSON(http.StatusOK, listModelCustomizationJobsOutput{
		ModelCustomizationJobSummaries: summaries,
		NextToken:                      outToken,
	})
}

func (h *Handler) handleStopModelCustomizationJob(c *echo.Context, id string) error {
	if err := h.Backend.StopModelCustomizationJob(id); err != nil {
		return h.writeError(c, err)
	}

	return c.NoContent(http.StatusOK)
}
