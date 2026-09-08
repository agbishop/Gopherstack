package dlm

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	matchPriority = service.PriorityPathVersioned

	pathPoliciesBase  = "/policies"
	pathTagsBase      = "/tags/"
	pathPoliciesSlash = "/policies/"

	opCreateLifecyclePolicy = "CreateLifecyclePolicy"
	opDeleteLifecyclePolicy = "DeleteLifecyclePolicy"
	opGetLifecyclePolicies  = "GetLifecyclePolicies"
	opGetLifecyclePolicy    = "GetLifecyclePolicy"
	opUpdateLifecyclePolicy = "UpdateLifecyclePolicy"
	opTagResource           = "TagResource"
	opUntagResource         = "UntagResource"
	opListTagsForResource   = "ListTagsForResource"
	opUnknown               = "Unknown"
)

// Handler handles DLM HTTP requests.
type Handler struct {
	Backend StorageBackend
}

// NewHandler constructs a new Handler.
func NewHandler(b StorageBackend) *Handler {
	return &Handler{Backend: b}
}

// Name returns the service name.
func (h *Handler) Name() string { return "DLM" }

// Reset resets the backend.
func (h *Handler) Reset() { h.Backend.Reset() }

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opCreateLifecyclePolicy,
		opDeleteLifecyclePolicy,
		opGetLifecyclePolicies,
		opGetLifecyclePolicy,
		opUpdateLifecyclePolicy,
		opTagResource,
		opUntagResource,
		opListTagsForResource,
	}
}

// RouteMatcher returns a matcher that accepts DLM REST paths.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path

		if strings.HasPrefix(path, pathTagsBase) {
			return isDLMResourceARN(path[len(pathTagsBase):])
		}

		return path == pathPoliciesBase ||
			strings.HasPrefix(path, pathPoliciesSlash)
	}
}

// isDLMResourceARN reports whether s looks like a DLM resource ARN, i.e.
// "arn:<partition>:dlm:...". The partition segment must not be
// hardcoded to "aws": pkgs/arn.Build derives it from the backend's region
// (aws-us-gov, aws-cn, aws-iso, aws-iso-b), so a hardcoded "arn:aws:dlm:"
// prefix check would silently reject every TagResource/UntagResource/
// ListTagsForResource request against a policy created in a non-standard
// partition -- the ARN this very backend generated in CreateLifecyclePolicy
// would then fail to route back into its own handler.
func isDLMResourceARN(s string) bool {
	// arn : partition : service : region : account : resource -- splitting
	// on the first arnSplitLimit-1 colons isolates the "service" segment
	// (index arnServiceSegment) without needing to parse the remainder
	// (region/account/resource, which may itself contain colons).
	const (
		arnServiceSegment = 2
		arnSplitLimit     = arnServiceSegment + 2
	)

	parts := strings.SplitN(s, ":", arnSplitLimit)

	return len(parts) > arnServiceSegment && parts[0] == "arn" && parts[arnServiceSegment] == "dlm"
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return matchPriority }

// ExtractOperation extracts the operation name from the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	return classifyPath(c.Request().Method, c.Request().URL.Path)
}

// ExtractResource extracts the resource identifier from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	path := c.Request().URL.Path
	if resource, ok := strings.CutPrefix(path, pathTagsBase); ok {
		return resource
	}

	if id, ok := strings.CutPrefix(path, pathPoliciesSlash); ok {
		return id
	}

	return path
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return h.handleREST(c)
	}
}

func (h *Handler) handleREST(c *echo.Context) error {
	ctx := c.Request().Context()
	log := logger.Load(ctx)

	path := c.Request().URL.Path
	method := c.Request().Method
	op := classifyPath(method, path)

	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return c.JSON(http.StatusBadRequest, errBody(errInvalidRequest, "failed to read body"))
	}

	switch op {
	case opCreateLifecyclePolicy:
		return h.handleCreateLifecyclePolicy(c, body)

	case opDeleteLifecyclePolicy:
		policyID, _ := strings.CutPrefix(path, pathPoliciesSlash)

		return h.handleDeleteLifecyclePolicy(c, policyID)

	case opGetLifecyclePolicies:
		return h.handleGetLifecyclePolicies(c)

	case opGetLifecyclePolicy:
		policyID, _ := strings.CutPrefix(path, pathPoliciesSlash)

		return h.handleGetLifecyclePolicy(c, policyID)

	case opUpdateLifecyclePolicy:
		policyID, _ := strings.CutPrefix(path, pathPoliciesSlash)

		return h.handleUpdateLifecyclePolicy(c, policyID, body)

	case opTagResource:
		resourceARN, _ := strings.CutPrefix(path, pathTagsBase)

		return h.handleTagResource(c, resourceARN, body)

	case opUntagResource:
		resourceARN, _ := strings.CutPrefix(path, pathTagsBase)

		return h.handleUntagResource(c, resourceARN, c.Request().URL.Query()["tagKeys"])

	case opListTagsForResource:
		resourceARN, _ := strings.CutPrefix(path, pathTagsBase)

		return h.handleListTagsForResource(c, resourceARN)

	default:
		log.Debug("dlm unknown operation", "method", method, "path", path)

		return c.JSON(http.StatusNotImplemented, errBody("NotImplementedException", "operation not implemented"))
	}
}

func classifyPath(method, path string) string {
	if strings.HasPrefix(path, pathTagsBase) {
		switch method {
		case http.MethodPost:
			return opTagResource
		case http.MethodDelete:
			return opUntagResource
		case http.MethodGet:
			return opListTagsForResource
		}

		return opUnknown
	}

	switch {
	case method == http.MethodPost && path == pathPoliciesBase:
		return opCreateLifecyclePolicy
	case method == http.MethodGet && path == pathPoliciesBase:
		return opGetLifecyclePolicies
	case method == http.MethodGet && strings.HasPrefix(path, pathPoliciesSlash):
		return opGetLifecyclePolicy
	case method == http.MethodDelete && strings.HasPrefix(path, pathPoliciesSlash):
		return opDeleteLifecyclePolicy
	case method == http.MethodPatch && strings.HasPrefix(path, pathPoliciesSlash):
		return opUpdateLifecyclePolicy
	}

	return opUnknown
}

func (h *Handler) handleCreateLifecyclePolicy(c *echo.Context, body []byte) error {
	var req struct {
		Tags                   map[string]string `json:"Tags"`
		PolicyDetails          map[string]any    `json:"PolicyDetails"`
		Exclusions             map[string]any    `json:"Exclusions"`
		CopyTags               *bool             `json:"CopyTags"`
		CreateInterval         *int32            `json:"CreateInterval"`
		RetainInterval         *int32            `json:"RetainInterval"`
		ExtendDeletion         *bool             `json:"ExtendDeletion"`
		Description            string            `json:"Description"`
		ExecutionRoleArn       string            `json:"ExecutionRoleArn"`
		State                  string            `json:"State"`
		DefaultPolicy          string            `json:"DefaultPolicy"`
		CrossRegionCopyTargets []any             `json:"CrossRegionCopyTargets"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return c.JSON(http.StatusBadRequest, errBody(errInvalidRequest, "invalid request body"))
	}

	defaultFields := defaultPolicyFields{
		DefaultPolicy:          req.DefaultPolicy,
		CopyTags:               req.CopyTags,
		CreateInterval:         req.CreateInterval,
		RetainInterval:         req.RetainInterval,
		ExtendDeletion:         req.ExtendDeletion,
		CrossRegionCopyTargets: req.CrossRegionCopyTargets,
		Exclusions:             req.Exclusions,
	}
	policyDetails := defaultFields.applyTo(req.PolicyDetails)

	policy, err := h.Backend.CreateLifecyclePolicy(
		req.Description, req.ExecutionRoleArn, req.State, req.Tags, policyDetails,
	)
	if err != nil {
		return h.mapError(c, err)
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"PolicyId": policy.PolicyID,
	})
}

func (h *Handler) handleDeleteLifecyclePolicy(c *echo.Context, policyID string) error {
	if err := h.Backend.DeleteLifecyclePolicy(policyID); err != nil {
		return h.mapError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

func (h *Handler) handleGetLifecyclePolicies(c *echo.Context) error {
	// The real client sends each list-valued filter as repeated query keys
	// (e.g. policyIds=a&policyIds=b), never comma-joined, so every value
	// under a given key must be read via q[key], not q.Get(key).
	q := c.Request().URL.Query()

	filter := PolicyFilter{
		PolicyIDs:         q["policyIds"],
		State:             q.Get("state"),
		ResourceTypes:     q["resourceTypes"],
		TargetTags:        q["targetTags"],
		TagsToAdd:         q["tagsToAdd"],
		DefaultPolicyType: q.Get("defaultPolicyType"),
	}

	summaries, err := h.Backend.GetLifecyclePolicies(filter)
	if err != nil {
		return h.mapError(c, err)
	}

	type summaryResp struct {
		Tags          map[string]string `json:"Tags,omitempty"`
		PolicyID      string            `json:"PolicyId"`
		Description   string            `json:"Description"`
		State         string            `json:"State"`
		PolicyType    string            `json:"PolicyType"`
		DefaultPolicy bool              `json:"DefaultPolicy"`
	}

	resp := make([]summaryResp, 0, len(summaries))
	for _, s := range summaries {
		resp = append(resp, summaryResp{
			PolicyID:      s.PolicyID,
			Description:   s.Description,
			State:         s.State,
			Tags:          s.Tags,
			PolicyType:    s.PolicyType,
			DefaultPolicy: s.DefaultPolicy,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"Policies": resp,
	})
}

func (h *Handler) handleGetLifecyclePolicy(c *echo.Context, policyID string) error {
	policy, err := h.Backend.GetLifecyclePolicy(policyID)
	if err != nil {
		return h.mapError(c, err)
	}

	policyDetails := map[string]any{
		"PolicyType": "EBS_SNAPSHOT_MANAGEMENT",
	}
	if policy.PolicyDetails != nil {
		policyDetails = policy.PolicyDetails
	}

	return c.JSON(http.StatusOK, map[string]any{
		"Policy": map[string]any{
			"PolicyId":         policy.PolicyID,
			"PolicyArn":        policy.PolicyArn,
			"Description":      policy.Description,
			"ExecutionRoleArn": policy.ExecutionRoleARN,
			"State":            policy.State,
			"StatusMessage":    policy.StatusMessage,
			"DefaultPolicy":    policy.DefaultPolicy,
			"DateCreated":      policy.DateCreated,
			"DateModified":     policy.DateModified,
			"Tags":             policy.Tags,
			"PolicyDetails":    policyDetails,
		},
	})
}

func (h *Handler) handleUpdateLifecyclePolicy(c *echo.Context, policyID string, body []byte) error {
	var req struct {
		PolicyDetails          map[string]any `json:"PolicyDetails"`
		Description            *string        `json:"Description"`
		ExecutionRoleArn       *string        `json:"ExecutionRoleArn"`
		Exclusions             map[string]any `json:"Exclusions"`
		CopyTags               *bool          `json:"CopyTags"`
		CreateInterval         *int32         `json:"CreateInterval"`
		RetainInterval         *int32         `json:"RetainInterval"`
		ExtendDeletion         *bool          `json:"ExtendDeletion"`
		State                  string         `json:"State"`
		CrossRegionCopyTargets []any          `json:"CrossRegionCopyTargets"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return c.JSON(http.StatusBadRequest, errBody(errInvalidRequest, "invalid request body"))
	}

	// UpdateLifecyclePolicyInput has no DefaultPolicy member (the policy
	// type can't change after creation) -- see defaultPolicyFields doc.
	defaultFields := defaultPolicyFields{
		CopyTags:               req.CopyTags,
		CreateInterval:         req.CreateInterval,
		RetainInterval:         req.RetainInterval,
		ExtendDeletion:         req.ExtendDeletion,
		CrossRegionCopyTargets: req.CrossRegionCopyTargets,
		Exclusions:             req.Exclusions,
	}

	if err := h.Backend.UpdateLifecyclePolicy(
		policyID, req.Description, req.ExecutionRoleArn, req.State, req.PolicyDetails, defaultFields.overrides(),
	); err != nil {
		return h.mapError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

func (h *Handler) handleTagResource(c *echo.Context, resourceARN string, body []byte) error {
	var req struct {
		Tags map[string]string `json:"Tags"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return c.JSON(http.StatusBadRequest, errBody(errInvalidRequest, "invalid request body"))
	}

	if err := h.Backend.TagResource(resourceARN, req.Tags); err != nil {
		return h.mapError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

func (h *Handler) handleUntagResource(c *echo.Context, resourceARN string, tagKeys []string) error {
	if err := h.Backend.UntagResource(resourceARN, tagKeys); err != nil {
		return h.mapError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

func (h *Handler) handleListTagsForResource(c *echo.Context, resourceARN string) error {
	tags, err := h.Backend.ListTagsForResource(resourceARN)
	if err != nil {
		return h.mapError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"Tags": tags,
	})
}

func (h *Handler) mapError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, awserr.ErrNotFound):
		return c.JSON(http.StatusNotFound, errBody(errResourceNotFound, err.Error()))
	case errors.Is(err, ErrLimitExceeded):
		return c.JSON(http.StatusBadRequest, errBody(errLimitExceeded, err.Error()))
	case errors.Is(err, awserr.ErrInvalidParameter):
		return c.JSON(http.StatusBadRequest, errBody(errInvalidRequest, err.Error()))
	default:
		return c.JSON(http.StatusInternalServerError, errBody("InternalServerException", err.Error()))
	}
}

func errBody(code, msg string) map[string]string {
	return map[string]string{
		"__type":  code,
		"message": msg,
	}
}
