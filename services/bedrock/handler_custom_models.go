package bedrock

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
)

func extractCustomModelOperation(path, method string) (string, bool) {
	if op, ok := extractCustomModelDeploymentSubOp(path, method); ok {
		return op, true
	}

	if method != http.MethodPost {
		return "", false
	}

	switch path {
	case customModelsCreate:
		return "CreateCustomModel", true
	case customModelDeploymentsPath:
		return "CreateCustomModelDeployment", true
	case foundationModelAgreement:
		return "CreateFoundationModelAgreement", true
	default:
		return "", false
	}
}

func (h *Handler) routeCustomModel(
	c *echo.Context,
	path, method string,
	body []byte,
) (bool, error) {
	if method != http.MethodPost {
		return false, nil
	}

	switch path {
	case customModelsCreate:
		return true, h.handleCreateCustomModel(c, body)
	case customModelDeploymentsPath:
		return true, h.handleCreateCustomModelDeployment(c, body)
	case foundationModelAgreement:
		return true, h.handleCreateFoundationModelAgreement(c, body)
	default:
		return false, nil
	}
}

type createCustomModelInput struct {
	ModelName string `json:"modelName"`
	Tags      []Tag  `json:"modelTags,omitempty"`
}

type createCustomModelOutput struct {
	ModelArn string `json:"modelArn"`
}

func (h *Handler) handleCreateCustomModel(c *echo.Context, body []byte) error {
	in, err := parseBody[createCustomModelInput](body)
	if err != nil {
		return c.JSON(
			http.StatusBadRequest,
			errorResponse("ValidationException", "invalid request body"),
		)
	}

	model, opErr := h.Backend.CreateCustomModel(in.ModelName, in.Tags)
	if opErr != nil {
		return h.writeError(c, opErr)
	}

	return c.JSON(http.StatusCreated, createCustomModelOutput{ModelArn: model.ModelArn})
}

func extractCustomModelListOperation(path, method string) (string, bool) {
	isSubPath := strings.HasPrefix(path, customModelsPrefix+"/")

	switch {
	case path == customModelsPrefix && method == http.MethodGet:
		return "ListCustomModels", true
	case isSubPath && method == http.MethodGet:
		return "GetCustomModel", true
	case isSubPath && method == http.MethodDelete:
		return "DeleteCustomModel", true
	default:
		return "", false
	}
}

func (h *Handler) routeCustomModelList(c *echo.Context, path, method string) (bool, error) {
	isSubPath := strings.HasPrefix(path, customModelsPrefix+"/")

	switch {
	case path == customModelsPrefix && method == http.MethodGet:
		return true, h.handleListCustomModels(c)
	case isSubPath && method == http.MethodGet:
		id := decodePath(strings.TrimPrefix(path, customModelsPrefix+"/"))

		return true, h.handleGetCustomModel(c, id)
	case isSubPath && method == http.MethodDelete:
		id := decodePath(strings.TrimPrefix(path, customModelsPrefix+"/"))

		return true, h.handleDeleteCustomModel(c, id)
	default:
		return false, nil
	}
}

// customModelOutput is GetCustomModel's response shape (bedrock@v1.66.4
// GetCustomModelResponse via botocore service-2.json): jobArn/jobName,
// baseModelArn, no baseModelName -- see customModelSummaryOutput for the
// distinct ListCustomModels shape.
type customModelOutput struct {
	CreationTime      string `json:"creationTime"`
	ModelArn          string `json:"modelArn"`
	ModelName         string `json:"modelName"`
	ModelStatus       string `json:"modelStatus,omitempty"`
	BaseModelArn      string `json:"baseModelArn,omitempty"`
	CustomizationType string `json:"customizationType,omitempty"`
	JobArn            string `json:"jobArn,omitempty"`
	JobName           string `json:"jobName,omitempty"`
	Tags              []Tag  `json:"tags,omitempty"`
}

func customModelToOutput(m *CustomModel) customModelOutput {
	return customModelOutput{
		ModelArn:          m.ModelArn,
		ModelName:         m.ModelName,
		ModelStatus:       m.ModelStatus,
		BaseModelArn:      m.BaseModelArn,
		CustomizationType: m.CustomizationType,
		JobArn:            m.JobArn,
		JobName:           m.JobName,
		CreationTime:      m.CreationTime.Format(time.RFC3339),
		Tags:              m.Tags,
	}
}

func (h *Handler) handleGetCustomModel(c *echo.Context, id string) error {
	m, err := h.Backend.GetCustomModel(id)
	if err != nil {
		return h.writeError(c, err)
	}

	return c.JSON(http.StatusOK, customModelToOutput(m))
}

// customModelSummaryOutput is ListCustomModels' per-item shape (bedrock@v1.66.4
// CustomModelSummary via botocore service-2.json): baseModelArn/baseModelName,
// no jobArn/jobName, distinct from Get's shape.
type customModelSummaryOutput struct {
	CreationTime      string `json:"creationTime"`
	ModelArn          string `json:"modelArn"`
	ModelName         string `json:"modelName"`
	ModelStatus       string `json:"modelStatus,omitempty"`
	BaseModelArn      string `json:"baseModelArn,omitempty"`
	BaseModelName     string `json:"baseModelName,omitempty"`
	CustomizationType string `json:"customizationType,omitempty"`
	Tags              []Tag  `json:"tags,omitempty"`
}

func customModelToSummaryOutput(m *CustomModel) customModelSummaryOutput {
	return customModelSummaryOutput{
		ModelArn:          m.ModelArn,
		ModelName:         m.ModelName,
		ModelStatus:       m.ModelStatus,
		BaseModelArn:      m.BaseModelArn,
		BaseModelName:     m.BaseModelName,
		CustomizationType: m.CustomizationType,
		CreationTime:      m.CreationTime.Format(time.RFC3339),
		Tags:              m.Tags,
	}
}

type listCustomModelsOutput struct {
	NextToken      string                     `json:"nextToken,omitempty"`
	ModelSummaries []customModelSummaryOutput `json:"modelSummaries"`
}

// parseListCustomModelsQuery builds the backend filter/sort/pagination input from
// the real ListCustomModels query-string bindings (aws-sdk-go-v2
// serializers.go:6152-6202): modelStatus, nameContains, isOwned,
// creationTimeAfter/Before, baseModelArnEquals, foundationModelArnEquals,
// sortBy, sortOrder, nextToken.
func parseListCustomModelsQuery(c *echo.Context) *ListCustomModelsInput {
	q := c.Request().URL.Query()

	maxResults, _ := strconv.ParseInt(q.Get("maxResults"), 10, 32)

	in := &ListCustomModelsInput{
		ModelStatus:              q.Get("modelStatus"),
		NameContains:             q.Get("nameContains"),
		BaseModelArnEquals:       q.Get("baseModelArnEquals"),
		FoundationModelArnEquals: q.Get("foundationModelArnEquals"),
		SortBy:                   q.Get("sortBy"),
		SortOrder:                q.Get("sortOrder"),
		NextToken:                q.Get("nextToken"),
		MaxResults:               int32(maxResults),
	}

	if v := q.Get("isOwned"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			in.IsOwned = &b
		}
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

func (h *Handler) handleListCustomModels(c *echo.Context) error {
	models, outToken := h.Backend.ListCustomModels(parseListCustomModelsQuery(c))
	summaries := make([]customModelSummaryOutput, 0, len(models))

	for _, m := range models {
		summaries = append(summaries, customModelToSummaryOutput(m))
	}

	return c.JSON(
		http.StatusOK,
		listCustomModelsOutput{ModelSummaries: summaries, NextToken: outToken},
	)
}

func (h *Handler) handleDeleteCustomModel(c *echo.Context, id string) error {
	if err := h.Backend.DeleteCustomModel(id); err != nil {
		return h.writeError(c, err)
	}

	return c.NoContent(http.StatusOK)
}
