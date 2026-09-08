package databrew

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	databrewPathPrefix = "/databrew/v1/"

	segDatasets       = "datasets"
	segRecipes        = "recipes"
	segRecipeVersions = "recipeVersions"
	segProjects       = "projects"
	segProfileJobs    = "profileJobs"
	segRecipeJobs     = "recipeJobs"
	segJobs           = "jobs"
	segRulesets       = "rulesets"
	segSchedules      = "schedules"
	segTags           = "tags"
	segJobRun         = "jobRun"
	nextTokenKey      = "NextToken"

	opCreateDataset   = "CreateDataset"
	opDescribeDataset = "DescribeDataset"
	opListDatasets    = "ListDatasets"
	opUpdateDataset   = "UpdateDataset"
	opDeleteDataset   = "DeleteDataset"
	opCreateRecipe    = "CreateRecipe"
	opDescribeRecipe  = "DescribeRecipe"
	opListRecipes     = "ListRecipes"
	opPublishRecipe   = "PublishRecipe"
	opUpdateRecipe    = "UpdateRecipe"
	// opDeleteRecipe is an internal route label for bare "DELETE /recipes/{name}".
	// It is NOT a real AWS DataBrew SDK operation — the real API has no DeleteRecipe
	// method at all (verified against botocore's databrew service-2.json: the only
	// DELETE recipe route is "DELETE /recipes/{name}/recipeVersion/{recipeVersion}",
	// i.e. DeleteRecipeVersion). To delete an entire recipe, a real client calls
	// BatchDeleteRecipeVersion with every version including LATEST_WORKING — there is
	// no single-shot delete-the-recipe call. parseRecipeOp checks the real
	// "recipeVersion/{version}" suffix first, so this fallback never shadows a real
	// client's DeleteRecipeVersion/BatchDeleteRecipeVersion call; it is kept wired as
	// internal test/tooling convenience only and deliberately excluded from
	// GetSupportedOperations() so gopherstack does not claim SDK support for an
	// operation AWS does not have — see gopherstack-vhw2 category A, and the same
	// resolution for CloudFront's GetFunctionAssociations/SetFunctionAssociations and
	// EMR's ListTagsForResource.
	opDeleteRecipe     = "DeleteRecipe"
	opCreateProject    = "CreateProject"
	opDescribeProject  = "DescribeProject"
	opListProjects     = "ListProjects"
	opUpdateProject    = "UpdateProject"
	opDeleteProject    = "DeleteProject"
	opCreateProfileJob = "CreateProfileJob"
	opCreateRecipeJob  = "CreateRecipeJob"
	opDescribeJob      = "DescribeJob"
	opListJobs         = "ListJobs"
	opUpdateProfileJob = "UpdateProfileJob"
	opUpdateRecipeJob  = "UpdateRecipeJob"
	opDeleteJob        = "DeleteJob"
	opStartJobRun      = "StartJobRun"
	opListJobRuns      = "ListJobRuns"
	opDescribeJobRun   = "DescribeJobRun"
	opStopJobRun       = "StopJobRun"

	opCreateRuleset   = "CreateRuleset"
	opDescribeRuleset = "DescribeRuleset"
	opListRulesets    = "ListRulesets"
	opUpdateRuleset   = "UpdateRuleset"
	opDeleteRuleset   = "DeleteRuleset"

	opCreateSchedule   = "CreateSchedule"
	opDescribeSchedule = "DescribeSchedule"
	opListSchedules    = "ListSchedules"
	opUpdateSchedule   = "UpdateSchedule"
	opDeleteSchedule   = "DeleteSchedule"

	opTagResource         = "TagResource"
	opUntagResource       = "UntagResource"
	opListTagsForResource = "ListTagsForResource"

	opBatchDeleteRecipeVersion = "BatchDeleteRecipeVersion"
	opDeleteRecipeVersion      = "DeleteRecipeVersion"
	opListRecipeVersions       = "ListRecipeVersions"

	opSendProjectSessionAction = "SendProjectSessionAction"
	opStartProjectSession      = "StartProjectSession"

	opUnknown = "Unknown"

	minPathSegments = 2

	keyName = "Name"
)

var (
	errUnknownAction  = errors.New("unknown action")
	errInvalidRequest = errors.New("invalid request body")
)

// Handler is the HTTP handler for AWS Glue DataBrew operations.
type Handler struct {
	Backend   StorageBackend
	AccountID string
	Region    string
}

// NewHandler creates a new DataBrew handler.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{
		Backend:   backend,
		AccountID: backend.AccountID(),
		Region:    backend.Region(),
	}
}

func (h *Handler) Name() string                        { return "DataBrew" }
func (h *Handler) Reset()                              { h.Backend.Reset() }
func (h *Handler) StartWorker(_ context.Context) error { return nil }

// Shutdown implements service.Shutdowner. It cancels in-flight job run
// transition goroutines and waits for them to drain, bounded by ctx.
func (h *Handler) Shutdown(ctx context.Context) {
	if b, ok := h.Backend.(interface{ Shutdown(context.Context) }); ok {
		b.Shutdown(ctx)
	}
}

var _ service.Shutdowner = (*Handler)(nil)

func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opCreateDataset, opDescribeDataset, opListDatasets, opUpdateDataset, opDeleteDataset,
		// opDeleteRecipe is deliberately NOT advertised — see its doc comment above;
		// it is not a real DataBrew SDK operation.
		opCreateRecipe, opDescribeRecipe, opListRecipes, opPublishRecipe, opUpdateRecipe,
		opCreateProject, opDescribeProject, opListProjects, opUpdateProject, opDeleteProject,
		opCreateProfileJob, opCreateRecipeJob, opDescribeJob, opListJobs,
		opUpdateProfileJob, opUpdateRecipeJob, opDeleteJob, opStartJobRun, opListJobRuns,
		opDescribeJobRun, opStopJobRun,
		opCreateRuleset, opDescribeRuleset, opListRulesets, opUpdateRuleset, opDeleteRuleset,
		opCreateSchedule, opDescribeSchedule, opListSchedules, opUpdateSchedule, opDeleteSchedule,
		opTagResource, opUntagResource, opListTagsForResource,
		opBatchDeleteRecipeVersion, opDeleteRecipeVersion, opListRecipeVersions,
		opSendProjectSessionAction, opStartProjectSession,
	}
}

func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path

		if strings.HasPrefix(c.Request().Host, "databrew.") ||
			strings.HasPrefix(path, databrewPathPrefix) ||
			path == "/databrew/v1" {
			return true
		}

		// Match paths sent by the SDK. Unambiguous segments match unconditionally;
		// ambiguous segments (datasets, schedules, jobs, tags, recipeVersions)
		// require a SigV4 service check. tags/{ResourceArn} and recipeVersions
		// are top-level real AWS paths (TagResource/UntagResource/
		// ListTagsForResource and ListRecipeVersions), not just the
		// /databrew/v1/ convenience prefix.
		firstSeg, _, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
		switch firstSeg {
		case segRecipes, segProfileJobs, segRecipeJobs, segRulesets, segProjects:
			return true
		case segDatasets, segSchedules, segJobs, segTags, segRecipeVersions:
			return httputils.ExtractServiceFromRequest(c.Request()) == "databrew"
		}

		return false
	}
}

// MatchPriority returns PriorityPathVersioned+1 so DataBrew is evaluated before
// IoT Analytics, which also claims /datasets at PriorityPathVersioned.
func (h *Handler) MatchPriority() int { return service.PriorityPathVersioned + 1 }

func (h *Handler) ExtractOperation(c *echo.Context) string {
	op, _ := parseDataBrewRESTPath(c.Request().Method, c.Request().URL.Path)

	return op
}

func (h *Handler) ExtractResource(c *echo.Context) string {
	_, name := parseDataBrewRESTPath(c.Request().Method, c.Request().URL.Path)

	return name
}

func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		// Resolve the per-request region (from SigV4 / X-Amz-Region) and attach
		// it to the context so backend operations are region-scoped.
		region := httputils.ExtractRegionFromRequest(c.Request(), h.Backend.Region())
		ctx = context.WithValue(ctx, regionContextKey{}, region)

		action, name := parseDataBrewRESTPath(c.Request().Method, c.Request().URL.Path)
		if action == opUnknown {
			return h.handleError(c, errUnknownAction)
		}

		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			log.ErrorContext(ctx, "databrew: failed to read request body", "error", err)

			return h.handleError(c, err)
		}

		body, err = enrichDataBrewBody(c, action, name, body)
		if err != nil {
			return h.handleError(c, err)
		}

		// For tags, job runs, and recipe versions, the resource identifier
		// travels as an extra path segment beyond resource/name; pull it into
		// the body so handlers can read it uniformly. This must run
		// unconditionally (not gated by a minimum segment count): every
		// DataBrew ARN embeds a "/" in its resource part (e.g.
		// "job/myjob"), so a bare (non-/databrew/v1/-prefixed) tag path like
		// /tags/arn:aws:databrew:us-east-1:111111111111:job/myjob only has 4
		// "/"-separated segments -- a fixed threshold tuned for the
		// convenience prefix would silently skip ResourceArn extraction for
		// real SDK traffic.
		body = enrichDataBrewSubOpBody(c.Request().URL.Path, body)

		result, dispErr := h.dispatch(ctx, action, body)
		if dispErr != nil {
			return h.handleError(c, dispErr)
		}

		if result == nil {
			return c.JSON(http.StatusOK, map[string]any{})
		}

		return c.JSONBlob(http.StatusOK, result)
	}
}

// databrewErrorResponse is the restjson1 error envelope. The real SDK's error
// deserializer (aws-sdk-go-v2/aws/protocol/restjson.GetErrorInfo) identifies
// the concrete exception type solely from the X-Amzn-ErrorType header or a
// "code"/"__type" JSON field -- HTTP status is NOT consulted. A body of just
// {"Message": "..."} (no code/__type) is silently downgraded by the SDK to a
// generic smithy.GenericAPIError, so callers doing errors.As(err,
// &types.ResourceNotFoundException{}) never match. __type must carry the
// exception name for typed error handling to work.
type databrewErrorResponse struct {
	Type    string `json:"__type"`
	Message string `json:"message"`
}

func (h *Handler) handleError(c *echo.Context, err error) error {
	status, code := http.StatusInternalServerError, "InternalFailure"

	switch {
	case errors.Is(err, ErrNotFound):
		status, code = http.StatusNotFound, errCodeResourceNotFound
	case errors.Is(err, ErrAlreadyExists):
		status, code = http.StatusConflict, errCodeConflict
	case errors.Is(err, ErrConflict):
		status, code = http.StatusConflict, errCodeConflict
	case errors.Is(err, ErrValidation):
		status, code = http.StatusBadRequest, errCodeValidation
	case errors.Is(err, errUnknownAction):
		status, code = http.StatusNotFound, errCodeResourceNotFound
	case errors.Is(err, errInvalidRequest):
		status, code = http.StatusBadRequest, errCodeValidation
	}

	return c.JSON(status, databrewErrorResponse{Type: code, Message: err.Error()})
}

func parseDataBrewRESTPath(method, path string) (string, string) {
	after, ok := strings.CutPrefix(path, databrewPathPrefix)
	if !ok {
		after, _ = strings.CutPrefix(path, "/")
	}

	segments := strings.SplitN(after, "/", minPathSegments+1)
	if len(segments) == 0 || segments[0] == "" {
		return opUnknown, ""
	}

	resource := segments[0]
	name := ""
	if len(segments) >= minPathSegments {
		name = segments[1]
	}

	var subOp string
	if len(segments) >= 3 { //nolint:mnd // 3 segments: resource/name/subOp
		subOp = segments[2]
	}

	return mapResourceOp(resource, method, name, subOp)
}

func mapResourceOp(resource, method, name, subOp string) (string, string) {
	switch resource {
	case segDatasets:

		return parseDatasetOp(method, name), name
	case segRecipes:

		return parseRecipeOp(method, name, subOp), name
	case segProjects:

		return parseProjectOp(method, name, subOp), name
	case segProfileJobs:

		return parseProfileJobOp(method, name), name
	case segRecipeJobs:

		return parseRecipeJobOp(method, name), name
	case segJobs:

		return parseJobOp(method, name, subOp), name
	case segRulesets:

		return parseRulesetOp(method, name), name
	case segSchedules:

		return parseScheduleOp(method, name), name
	case segTags:

		return parseTagsOp(method, name), name
	case segRecipeVersions:
		// The real AWS ListRecipeVersions op is GET /recipeVersions?name=...
		// (RecipeName travels as a query param, not a path segment; see
		// enrichDataBrewBody). BatchDeleteRecipeVersion is a recipe sub-op at
		// POST /recipes/{Name}/batchDeleteRecipeVersion (see
		// parseRecipeSubOp) -- there is no real AWS op at POST /recipeVersions.
		if method == http.MethodGet {
			return opListRecipeVersions, name
		}

		return opUnknown, ""
	}

	return opUnknown, ""
}

// onceSimpleQueryParamToJSONKey lazily initialises the query-string-param ->
// JSON-body-key lookup table exactly once, for the DataBrew query params
// that are plain pass-through strings (no special-cased merge/derivation
// logic) -- see mergeSimpleQueryParams.
//
//nolint:gochecknoglobals // read-only package-level lookup table
var onceSimpleQueryParamToJSONKey = sync.OnceValue(func() map[string]string {
	return map[string]string{
		"maxResults":    "MaxResults",
		"nextToken":     nextTokenKey,
		"datasetName":   "DatasetName",
		"projectName":   "ProjectName",
		"targetArn":     "TargetArn",
		"recipeVersion": "RecipeVersion",
	}
})

// mergeSimpleQueryParams copies every onceSimpleQueryParamToJSONKey entry
// present on the request into m.
func mergeSimpleQueryParams(c *echo.Context, m map[string]json.RawMessage) {
	for param, jsonKey := range onceSimpleQueryParamToJSONKey() {
		if v := c.QueryParam(param); v != "" {
			m[jsonKey], _ = json.Marshal(v)
		}
	}
}

func enrichDataBrewBody(c *echo.Context, _, name string, body []byte) ([]byte, error) {
	m := make(map[string]json.RawMessage)
	if len(body) > 0 {
		if err := json.Unmarshal(body, &m); err != nil {
			return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
		}
	}

	if name != "" {
		if existingName, ok := m["Name"]; ok {
			var parsedName string
			if err := json.Unmarshal(existingName, &parsedName); err == nil && parsedName != name {
				return nil, fmt.Errorf("%w: name in body does not match path", errInvalidRequest)
			}
		}
		nameJSON, _ := json.Marshal(name)
		m["Name"] = nameJSON
	}

	mergeSimpleQueryParams(c, m)

	// UntagResource is DELETE /tags/{ResourceArn}?tagKeys=a&tagKeys=b -- TagKeys
	// travels as a repeated query param, never in the (typically absent) DELETE
	// body. Confirmed against aws-sdk-go-v2's serializer
	// (awsRestjson1_serializeOpHttpBindingsUntagResourceInput calls
	// encoder.AddQuery("tagKeys") per key). Without this, UntagResource always
	// silently no-ops.
	if tagKeys := c.QueryParams()["tagKeys"]; len(tagKeys) > 0 {
		m["TagKeys"], _ = json.Marshal(tagKeys)
	}
	// ListRecipeVersions is GET /recipeVersions?name=... (top-level, no path
	// segment for the recipe name); only set when name isn't already known
	// from a path segment (e.g. the /recipes/{Name}/recipeVersions convenience
	// alias already populated "Name" above).
	if _, ok := m["Name"]; !ok {
		if v := c.QueryParam("name"); v != "" {
			m["Name"], _ = json.Marshal(v)
		}
	}

	result, _ := json.Marshal(m)

	return result, nil
}

func enrichDataBrewSubOpBody(path string, body []byte) []byte {
	segments := strings.Split(path, "/")
	m := make(map[string]json.RawMessage)
	if len(body) > 0 {
		_ = json.Unmarshal(body, &m)
	}

	for i, seg := range segments {
		switch seg {
		case "jobRun":
			// e.g. /jobs/{Name}/jobRun/{RunId} — with or without /databrew/v1/ prefix
			if i+1 < len(segments) {
				runIDJSON, _ := json.Marshal(segments[i+1])
				m["RunId"] = runIDJSON
			}
		case "recipeVersion":
			// e.g. /recipes/{Name}/recipeVersion/{RecipeVersion}
			if i+1 < len(segments) {
				versionJSON, _ := json.Marshal(segments[i+1])
				m["RecipeVersion"] = versionJSON
			}
		case "tags":
			// e.g. /databrew/v1/tags/{resourceArn} or /tags/{resourceArn}
			if i+1 < len(segments) {
				arn := strings.Join(segments[i+1:], "/")
				arnJSON, _ := json.Marshal(arn)
				m["ResourceArn"] = arnJSON
			}
		}
	}

	result, _ := json.Marshal(m)

	return result
}

func (h *Handler) dispatch(ctx context.Context, action string, body []byte) ([]byte, error) {
	if result, ok, err := h.dispatchDataset(ctx, action, body); ok {
		return result, err
	}

	if result, ok, err := h.dispatchRecipe(ctx, action, body); ok {
		return result, err
	}

	if result, ok, err := h.dispatchProject(ctx, action, body); ok {
		return result, err
	}

	if result, ok, err := h.dispatchJob(ctx, action, body); ok {
		return result, err
	}

	if result, ok, err := h.dispatchRuleset(ctx, action, body); ok {
		return result, err
	}

	if result, ok, err := h.dispatchSchedule(ctx, action, body); ok {
		return result, err
	}

	if result, ok, err := h.dispatchTags(ctx, action, body); ok {
		return result, err
	}

	return nil, fmt.Errorf("%w: %s", errUnknownAction, action)
}
