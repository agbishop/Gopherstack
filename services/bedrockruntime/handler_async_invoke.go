package bedrockruntime

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
)

// startAsyncInvokeInput is the parsed request body for StartAsyncInvoke.
// Note: the real StartAsyncInvokeInput.ModelInput member (an opaque,
// model-specific smithy document) is intentionally not modeled here --
// gopherstack cannot interpret arbitrary model input schemas, and the real
// AWS SDK client itself enforces ModelInput's presence before ever sending
// the request (client-side required-member validation), so a raw HTTP
// request that omits it is not a realistic scenario an SDK-driven caller can
// produce.
type startAsyncInvokeInput struct {
	Tags             map[string]string `json:"tags"`
	OutputDataConfig struct {
		S3OutputDataConfig struct {
			S3URI string `json:"s3Uri"`
		} `json:"s3OutputDataConfig"`
	} `json:"outputDataConfig"`
	ModelID                    string `json:"modelId"`
	ClientRequestToken         string `json:"clientRequestToken"`
	InferenceProfileIdentifier string `json:"inferenceProfileIdentifier,omitempty"`
}

// handleStartAsyncInvoke handles POST /async-invoke.
func (h *Handler) handleStartAsyncInvoke(c *echo.Context, body []byte) error {
	var req startAsyncInvokeInput

	if err := json.Unmarshal(body, &req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse("ValidationException", "invalid request body"))
	}

	s3URI := req.OutputDataConfig.S3OutputDataConfig.S3URI
	if s3URI != "" && !strings.HasPrefix(s3URI, "s3://") {
		return c.JSON(
			http.StatusBadRequest,
			errorResponse("ValidationException", "outputDataConfig.s3OutputDataConfig.s3Uri must start with s3://"),
		)
	}

	effectiveModelID := req.ModelID
	if effectiveModelID == "" && req.InferenceProfileIdentifier != "" {
		effectiveModelID = req.InferenceProfileIdentifier
	}

	inv, err := h.Backend.StartAsyncInvoke(
		effectiveModelID,
		s3URI,
		req.ClientRequestToken,
		req.Tags,
	)
	if err != nil {
		return handleError(c, err)
	}

	resp := map[string]any{
		keyInvocationArn: inv.InvocationArn,
	}

	c.Response().Header().Set("Content-Type", contentTypeJSON)

	return c.JSON(http.StatusAccepted, resp)
}

// handleGetAsyncInvoke handles GET /async-invoke/{invocationArn}.
func (h *Handler) handleGetAsyncInvoke(c *echo.Context, path string) error {
	invocationArn, _ := strings.CutPrefix(path, asyncInvokeItemPathPrefix)
	if invocationArn == "" {
		return c.JSON(
			http.StatusBadRequest,
			errorResponse("ValidationException", "missing invocationArn in path"),
		)
	}

	inv, err := h.Backend.GetAsyncInvoke(invocationArn)
	if err != nil {
		return handleError(c, err)
	}

	resp := buildAsyncInvokeResponse(inv)

	c.Response().Header().Set("Content-Type", contentTypeJSON)

	return c.JSON(http.StatusOK, resp)
}

// defaultListAsyncInvokesMaxResults matches the real API's page size when maxResults is
// omitted (bedrockruntime@v1.57.1 api_op_ListAsyncInvokes.go doc comment: "default 10").
const defaultListAsyncInvokesMaxResults = 10

// handleListAsyncInvokes handles GET /async-invoke.
// Supports query parameters: statusEquals (InProgress|Completed|Failed), maxResults,
// nextToken (both httpQuery-bound in ListAsyncInvokesInput -- serializers.go:1120-1126).
// parseListAsyncInvokesFilter reads the query bindings ListAsyncInvokes
// serializes: statusEquals, submitTimeAfter, submitTimeBefore and sortOrder
// (serializers.go:1129-1145). An unparseable timestamp is ignored rather than
// rejected -- the op models no validation error.
func parseListAsyncInvokesFilter(c *echo.Context) ListAsyncInvokesFilter {
	filter := ListAsyncInvokesFilter{
		StatusEquals: c.QueryParam("statusEquals"),
		SortOrder:    c.QueryParam("sortOrder"),
	}

	if t, err := time.Parse(time.RFC3339, c.QueryParam("submitTimeAfter")); err == nil {
		filter.SubmitTimeAfter = &t
	}

	if t, err := time.Parse(time.RFC3339, c.QueryParam("submitTimeBefore")); err == nil {
		filter.SubmitTimeBefore = &t
	}

	return filter
}

func (h *Handler) handleListAsyncInvokes(c *echo.Context) error {
	invocations := h.Backend.ListAsyncInvokes(parseListAsyncInvokesFilter(c))

	maxResults := defaultListAsyncInvokesMaxResults
	if s := c.QueryParam("maxResults"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxResults = n
		}
	}

	nextToken := c.QueryParam("nextToken")
	if nextToken != "" {
		idx := -1
		for i, inv := range invocations {
			if inv.InvocationArn == nextToken {
				idx = i

				break
			}
		}

		if idx >= 0 {
			invocations = invocations[idx+1:]
		} else {
			invocations = nil
		}
	}

	respToken := ""
	if len(invocations) > maxResults {
		respToken = invocations[maxResults-1].InvocationArn
		invocations = invocations[:maxResults]
	}

	summaries := make([]map[string]any, 0, len(invocations))

	for _, inv := range invocations {
		summaries = append(summaries, buildAsyncInvokeResponse(inv))
	}

	resp := map[string]any{
		"asyncInvokeSummaries": summaries,
	}
	if respToken != "" {
		resp["nextToken"] = respToken
	}

	c.Response().Header().Set("Content-Type", contentTypeJSON)

	return c.JSON(http.StatusOK, resp)
}

// buildAsyncInvokeResponse constructs the JSON response for a single async invocation.
// Fields that are only valid in terminal states (Completed/Failed) are omitted otherwise.
// Deliberately no "tags" key: neither GetAsyncInvokeOutput nor AsyncInvokeSummary
// (bedrockruntime@v1.57.1 api_op_GetAsyncInvoke.go / types/types.go) has a Tags
// member, and this service has no TagResource/ListTagsForResource op at all --
// real AWS gives no way to read an async invoke's tags back, ever.
func buildAsyncInvokeResponse(inv *AsyncInvoke) map[string]any {
	resp := map[string]any{
		keyInvocationArn: inv.InvocationArn,
		"modelArn":       inv.ModelArn,
		"outputDataConfig": map[string]any{
			"s3OutputDataConfig": map[string]any{
				"s3Uri": inv.OutputS3URI,
			},
		},
		"status":           inv.Status,
		"submitTime":       inv.SubmitTime.Format(time.RFC3339),
		"lastModifiedTime": inv.LastModifiedTime.Format(time.RFC3339),
	}

	if inv.ClientRequestToken != nil {
		resp["clientRequestToken"] = *inv.ClientRequestToken
	}

	// EndTime and failureMessage are only meaningful after the invocation has finished.
	isTerminal := inv.Status == AsyncInvokeStatusCompleted || inv.Status == AsyncInvokeStatusFailed

	if isTerminal && inv.EndTime != nil {
		resp["endTime"] = inv.EndTime.Format(time.RFC3339)
	}

	if inv.Status == AsyncInvokeStatusFailed && inv.FailureMessage != nil {
		resp["failureMessage"] = *inv.FailureMessage
	}

	return resp
}
