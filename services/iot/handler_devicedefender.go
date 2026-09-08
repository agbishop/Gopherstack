package iot

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
)

const (
	keyTaskStatus = "taskStatus"
	keyTasksField = "tasks"
)

// ---------------------------------------------------------------------------
// Route resolvers: Device Defender audit + detect mitigation-action tasks,
// violations, and audit-finding related resources.
// ---------------------------------------------------------------------------

func resolveDeviceDefenderOps(path, method string) string {
	if op := resolveAuditMitigationTaskOps(path, method); op != unknownOperation {
		return op
	}
	if op := resolveDetectMitigationTaskOps(path, method); op != unknownOperation {
		return op
	}
	if op := resolveViolationOps(path, method); op != unknownOperation {
		return op
	}

	if path == "/audit/relatedResources" && method == http.MethodGet {
		return opListRelatedResourcesForAuditFinding
	}

	return unknownOperation
}

const pathAuditMitigationTasks = "/audit/mitigationactions/tasks"

func resolveAuditMitigationTaskOps(path, method string) string {
	switch {
	case path == pathAuditMitigationTasks && method == http.MethodGet:
		return opListAuditMitigationActionsTasks
	case path == "/audit/mitigationactions/executions" && method == http.MethodGet:
		return opListAuditMitigationActionsExecutions
	// The "/cancel" suffix is handled by resolveJobAndAuditOps
	// (opCancelAuditMitigationActionsTask); skip it here.
	case strings.HasPrefix(path, pathAuditMitigationTasks+"/") &&
		!strings.HasSuffix(path, "/cancel") &&
		method == http.MethodPost:
		return opStartAuditMitigationActionsTask
	case strings.HasPrefix(path, pathAuditMitigationTasks+"/") &&
		!strings.HasSuffix(path, "/cancel") &&
		method == http.MethodGet:
		return opDescribeAuditMitigationActionsTask
	}

	return unknownOperation
}

const pathDetectMitigationTasks = "/detect/mitigationactions/tasks"

func resolveDetectMitigationTaskOps(path, method string) string {
	switch {
	case path == pathDetectMitigationTasks && method == http.MethodGet:
		return opListDetectMitigationActionsTasks
	case path == "/detect/mitigationactions/executions" && method == http.MethodGet:
		return opListDetectMitigationActionsExecutions
	case strings.HasPrefix(path, pathDetectMitigationTasks+"/") &&
		strings.HasSuffix(path, "/cancel") &&
		method == http.MethodPut:
		return opCancelDetectMitigationActionsTask
	case strings.HasPrefix(path, pathDetectMitigationTasks+"/") &&
		!strings.HasSuffix(path, "/cancel") &&
		method == http.MethodPut:
		return opStartDetectMitigationActionsTask
	case strings.HasPrefix(path, pathDetectMitigationTasks+"/") &&
		!strings.HasSuffix(path, "/cancel") &&
		method == http.MethodGet:
		return opDescribeDetectMitigationActionsTask
	}

	return unknownOperation
}

func resolveViolationOps(path, method string) string {
	switch {
	case path == "/active-violations" && method == http.MethodGet:
		return opListActiveViolations
	case path == "/violation-events" && method == http.MethodGet:
		return opListViolationEvents
	case strings.HasPrefix(path, "/violations/verification-state/") && method == http.MethodPost:
		return opPutVerificationStateOnViolation
	}

	return unknownOperation
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

func (h *Handler) dispatchDeviceDefenderOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opStartAuditMitigationActionsTask:
		return true, h.handleStartAuditMitigationActionsTask(c)
	case opDescribeAuditMitigationActionsTask:
		return true, h.handleDescribeAuditMitigationActionsTask(c)
	case opListAuditMitigationActionsTasks:
		return true, h.handleListAuditMitigationActionsTasks(c)
	case opListAuditMitigationActionsExecutions:
		return true, h.handleListAuditMitigationActionsExecutions(c)
	case opStartDetectMitigationActionsTask:
		return true, h.handleStartDetectMitigationActionsTask(c)
	case opDescribeDetectMitigationActionsTask:
		return true, h.handleDescribeDetectMitigationActionsTask(c)
	case opListDetectMitigationActionsTasks:
		return true, h.handleListDetectMitigationActionsTasks(c)
	case opListDetectMitigationActionsExecutions:
		return true, h.handleListDetectMitigationActionsExecutions(c)
	case opCancelDetectMitigationActionsTask:
		return true, h.handleCancelDetectMitigationActionsTask(c)
	case opListActiveViolations:
		return true, h.handleListActiveViolations(c)
	case opListViolationEvents:
		return true, h.handleListViolationEvents(c)
	case opPutVerificationStateOnViolation:
		return true, h.handlePutVerificationStateOnViolation(c)
	case opListRelatedResourcesForAuditFinding:
		return true, h.handleListRelatedResourcesForAuditFinding(c)
	}

	return false, nil
}

// ---------------------------------------------------------------------------
// Handlers: Audit mitigation-action tasks
// ---------------------------------------------------------------------------

func (h *Handler) handleStartAuditMitigationActionsTask(c *echo.Context) error {
	taskID := strings.TrimPrefix(c.Request().URL.Path, pathAuditMitigationTasks+"/")

	var req struct {
		Target                     *AuditMitigationActionsTaskTarget `json:"target"`
		AuditCheckToActionsMapping map[string][]string               `json:"auditCheckToActionsMapping"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}

	task, err := h.Backend.StartAuditMitigationActionsTask(&StartAuditMitigationActionsTaskInput{
		TaskID:                     taskID,
		Target:                     req.Target,
		AuditCheckToActionsMapping: req.AuditCheckToActionsMapping,
	})
	if err != nil {
		return respondAsConflictCode(c, err, ErrAlreadyExists, "TaskAlreadyExistsException")
	}

	return c.JSON(http.StatusOK, map[string]any{keyTaskID: task.TaskID})
}

// auditMitigationTaskActionsDefinition resolves AuditCheckToActionsMapping's
// per-check action-name lists to the real DescribeAuditMitigationActionsTaskOutput
// "actionsDefinition" member -- []types.MitigationAction (id/name/roleArn/
// actionParams, v1.77.4) -- the same field DescribeDetectMitigationActionsTask
// already surfaces via MitigationActionRefs for its own (flat) Actions list.
// This side was never given the same treatment: a real client's deserializer
// never found "actionsDefinition" and it stayed permanently empty.
func (h *Handler) auditMitigationTaskActionsDefinition(t *AuditMitigationTask) []MitigationActionRef {
	seen := make(map[string]bool)
	names := make([]string, 0, len(t.AuditCheckToActionsMapping))
	for _, checkNames := range t.AuditCheckToActionsMapping {
		for _, name := range checkNames {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)

	return h.Backend.MitigationActionRefs(names)
}

func (h *Handler) handleDescribeAuditMitigationActionsTask(c *echo.Context) error {
	taskID := strings.TrimPrefix(c.Request().URL.Path, pathAuditMitigationTasks+"/")

	task, err := h.Backend.DescribeAuditMitigationActionsTask(taskID)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyTarget:                    task.Target,
		"auditCheckToActionsMapping": task.AuditCheckToActionsMapping,
		"actionsDefinition":          h.auditMitigationTaskActionsDefinition(task),
		"taskStatistics":             task.TaskStatistics,
		"taskStatus":                 task.TaskStatus,
		"startTime":                  task.StartTime,
		"endTime":                    task.EndTime,
	})
}

// handleListAuditMitigationActionsTasks builds
// types.AuditMitigationActionsTaskMetadata's wire shape (v1.77.4):
// {taskId, taskStatus, startTime} only — a narrower summary than
// DescribeAuditMitigationActionsTaskOutput's, unlike the detect-mitigation
// side below. No "endTime" key; not present on the real type.
func (h *Handler) handleListAuditMitigationActionsTasks(c *echo.Context) error {
	auditTaskID := c.QueryParam("auditTaskId")
	taskStatus := c.QueryParam(keyTaskStatus)

	tasks := h.Backend.ListAuditMitigationActionsTasks(auditTaskID, taskStatus)

	summaries := make([]map[string]any, 0, len(tasks))
	for _, t := range tasks {
		summaries = append(summaries, map[string]any{
			keyTaskID:     t.TaskID,
			keyTaskStatus: t.TaskStatus,
			"startTime":   t.StartTime,
		})
	}

	pageSize, start := parseIoTPagination(c)
	page, nextToken := paginateMaps(summaries, pageSize, start)

	resp := map[string]any{keyTasksField: page}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleListAuditMitigationActionsExecutions(c *echo.Context) error {
	taskID := c.QueryParam(keyTaskID)
	findingID := c.QueryParam("findingId")
	actionStatus := c.QueryParam("actionStatus")

	execs := h.Backend.ListAuditMitigationActionsExecutions(taskID, findingID, actionStatus)

	pageSize, start := parseIoTPagination(c)
	page, nextToken := paginateMaps(execs, pageSize, start)

	resp := map[string]any{"actionsExecutions": page}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

// Handlers: Detect mitigation-action tasks.

func (h *Handler) handleStartDetectMitigationActionsTask(c *echo.Context) error {
	taskID := strings.TrimPrefix(c.Request().URL.Path, pathDetectMitigationTasks+"/")

	var req struct {
		Target                        *DetectMitigationActionsTaskTarget `json:"target"`
		ViolationEventOccurrenceRange *ViolationEventOccurrenceRange     `json:"violationEventOccurrenceRange"`
		Actions                       []string                           `json:"actions"`
		IncludeOnlyActiveViolations   bool                               `json:"includeOnlyActiveViolations"`
		IncludeSuppressedAlerts       bool                               `json:"includeSuppressedAlerts"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}

	task, err := h.Backend.StartDetectMitigationActionsTask(&StartDetectMitigationActionsTaskInput{
		TaskID:                        taskID,
		Target:                        req.Target,
		Actions:                       req.Actions,
		IncludeOnlyActiveViolations:   req.IncludeOnlyActiveViolations,
		IncludeSuppressedAlerts:       req.IncludeSuppressedAlerts,
		ViolationEventOccurrenceRange: req.ViolationEventOccurrenceRange,
	})
	if err != nil {
		return respondAsConflictCode(c, err, ErrAlreadyExists, "TaskAlreadyExistsException")
	}

	return c.JSON(http.StatusOK, map[string]any{keyTaskID: task.TaskID})
}

// detectMitigationTaskSummaryWire builds the real
// DetectMitigationActionsTaskSummary wire shape (aws-sdk-go-v2/service/iot/types@v1.77.4).
// Shared by DescribeDetectMitigationActionsTask and
// ListDetectMitigationActionsTasks, which use the same rich element type —
// unlike the audit-mitigation side's narrower list summary — so must NOT be
// reduced to a hand-picked subset of fields.
func (h *Handler) detectMitigationTaskSummaryWire(t *DetectMitigationTask) map[string]any {
	return map[string]any{
		"taskId":                        t.TaskID,
		"taskStatus":                    t.TaskStatus,
		keyTarget:                       t.Target,
		"taskStatistics":                t.TaskStatistics,
		"actionsDefinition":             h.Backend.MitigationActionRefs(t.Actions),
		"taskStartTime":                 t.StartTime,
		"taskEndTime":                   t.EndTime,
		"onlyActiveViolationsIncluded":  t.OnlyActiveViolationsIncluded,
		"suppressedAlertsIncluded":      t.SuppressedAlertsIncluded,
		"violationEventOccurrenceRange": t.ViolationEventOccurrenceRange,
	}
}

func (h *Handler) handleDescribeDetectMitigationActionsTask(c *echo.Context) error {
	taskID := strings.TrimPrefix(c.Request().URL.Path, pathDetectMitigationTasks+"/")

	task, err := h.Backend.DescribeDetectMitigationActionsTask(taskID)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"taskSummary": h.detectMitigationTaskSummaryWire(task)})
}

// handleListDetectMitigationActionsTasks: real AWS's
// ListDetectMitigationActionsTasksOutput.Tasks is
// []types.DetectMitigationActionsTaskSummary, the same rich type
// DescribeDetectMitigationActionsTask returns (v1.77.4), not a narrower
// list-only summary — hence sharing detectMitigationTaskSummaryWire.
func (h *Handler) handleListDetectMitigationActionsTasks(c *echo.Context) error {
	startTime := parseIoTEpochQueryParam(c, "startTime")
	endTime := parseIoTEpochQueryParam(c, "endTime")

	tasks := h.Backend.ListDetectMitigationActionsTasks(startTime, endTime)

	summaries := make([]map[string]any, 0, len(tasks))
	for _, t := range tasks {
		summaries = append(summaries, h.detectMitigationTaskSummaryWire(t))
	}

	pageSize, start := parseIoTPagination(c)
	page, nextToken := paginateMaps(summaries, pageSize, start)

	resp := map[string]any{keyTasksField: page}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleCancelDetectMitigationActionsTask(c *echo.Context) error {
	after := strings.TrimPrefix(c.Request().URL.Path, pathDetectMitigationTasks+"/")
	taskID, _, _ := strings.Cut(after, "/")

	if err := h.Backend.CancelDetectMitigationActionsTask(taskID); err != nil {
		return respondErr(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleListDetectMitigationActionsExecutions(c *echo.Context) error {
	taskID := c.QueryParam(keyTaskID)
	violationID := c.QueryParam("violationId")
	thingName := c.QueryParam("thingName")

	execs := h.Backend.ListDetectMitigationActionsExecutions(taskID, violationID, thingName)

	pageSize, start := parseIoTPagination(c)
	page, nextToken := paginateMaps(execs, pageSize, start)

	resp := map[string]any{"actionsExecutions": page}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Handlers: Violations
// ---------------------------------------------------------------------------

func (h *Handler) handleListActiveViolations(c *echo.Context) error {
	thingName := c.QueryParam("thingName")
	securityProfileName := c.QueryParam("securityProfileName")
	verificationState := c.QueryParam("verificationState")
	listSuppressedAlerts := parseIoTBoolQueryParam(c, "listSuppressedAlerts")
	behaviorCriteriaType := c.QueryParam("behaviorCriteriaType")

	violations := h.Backend.ListActiveViolations(
		thingName,
		securityProfileName,
		verificationState,
		listSuppressedAlerts,
		behaviorCriteriaType,
	)

	pageSize, start := parseIoTPagination(c)
	page, nextToken := paginateMaps(violations, pageSize, start)

	resp := map[string]any{"activeViolations": page}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleListViolationEvents(c *echo.Context) error {
	thingName := c.QueryParam("thingName")
	securityProfileName := c.QueryParam("securityProfileName")
	verificationState := c.QueryParam("verificationState")
	startTime := parseIoTEpochQueryParam(c, "startTime")
	endTime := parseIoTEpochQueryParam(c, "endTime")
	listSuppressedAlerts := parseIoTBoolQueryParam(c, "listSuppressedAlerts")
	behaviorCriteriaType := c.QueryParam("behaviorCriteriaType")

	events := h.Backend.ListViolationEvents(
		thingName, securityProfileName, verificationState, startTime, endTime,
		listSuppressedAlerts, behaviorCriteriaType,
	)

	pageSize, start := parseIoTPagination(c)
	page, nextToken := paginateMaps(events, pageSize, start)

	resp := map[string]any{"violationEvents": page}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handlePutVerificationStateOnViolation(c *echo.Context) error {
	violationID := strings.TrimPrefix(c.Request().URL.Path, "/violations/verification-state/")

	var req struct {
		VerificationState            string `json:"verificationState"`
		VerificationStateDescription string `json:"verificationStateDescription"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}

	if req.VerificationState == "" {
		return c.JSON(
			http.StatusBadRequest,
			awsErrBody{errTypeInvalidRequest, "verificationState is required"},
		)
	}

	err := h.Backend.PutVerificationStateOnViolation(
		violationID,
		req.VerificationState,
		req.VerificationStateDescription,
	)
	if err != nil {
		// PutVerificationStateOnViolation's own deserializeOpError switch
		// declares no ResourceNotFoundException case.
		return respondAsInvalidRequest(c, err, ErrResourceNotFound)
	}

	return c.NoContent(http.StatusOK)
}

// ---------------------------------------------------------------------------
// Handlers: Audit finding related resources
// ---------------------------------------------------------------------------

func (h *Handler) handleListRelatedResourcesForAuditFinding(c *echo.Context) error {
	findingID := c.QueryParam("findingId")

	resources, err := h.Backend.ListRelatedResourcesForAuditFinding(findingID)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"relatedResources": resources})
}

// parseIoTEpochQueryParam parses a query parameter as a float64 epoch-seconds
// timestamp, returning 0 (unbounded) if absent or invalid.
func parseIoTEpochQueryParam(c *echo.Context, name string) float64 {
	v := c.QueryParam(name)
	if v == "" {
		return 0
	}

	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}

	return f
}

// parseIoTBoolQueryParam parses a query parameter as a tri-state *bool,
// returning nil (unset/unfiltered) if absent or invalid.
func parseIoTBoolQueryParam(c *echo.Context, name string) *bool {
	v := c.QueryParam(name)
	if v == "" {
		return nil
	}

	b, err := strconv.ParseBool(v)
	if err != nil {
		return nil
	}

	return &b
}

func (h *Handler) handleCancelAuditMitigationActionsTask(c *echo.Context) error {
	// Path: /audit/mitigationactions/tasks/{taskId}/cancel
	after := strings.TrimPrefix(c.Request().URL.Path, "/audit/mitigationactions/tasks/")
	taskID, _, _ := strings.Cut(after, "/")

	if err := h.Backend.CancelAuditMitigationActionsTask(&CancelAuditMitigationActionsTaskInput{
		TaskID: taskID,
	}); err != nil {
		return h.handleError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleGetBehaviorModelTrainingSummaries(c *echo.Context) error {
	securityProfileName := c.QueryParam(keySecurityProfileName)
	maxResults := parseInt32QueryParam(c, "maxResults")
	nextToken := c.QueryParam("nextToken")

	summaries, next, err := h.Backend.GetBehaviorModelTrainingSummaries(securityProfileName, maxResults, nextToken)
	if err != nil {
		return respondErr(c, err)
	}

	resp := map[string]any{"summaries": summaries}
	if next != "" {
		resp["nextToken"] = next
	}

	return c.JSON(http.StatusOK, resp)
}
