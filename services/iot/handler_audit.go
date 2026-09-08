package iot

import (
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
)

func (h *Handler) handleCancelAuditTask(c *echo.Context) error {
	// Path: /audit/tasks/{taskId}/cancel
	after := strings.TrimPrefix(c.Request().URL.Path, "/audit/tasks/")
	taskID, _, _ := strings.Cut(after, "/")

	if err := h.Backend.CancelAuditTask(&CancelAuditTaskInput{
		AuditTaskID: taskID,
	}); err != nil {
		return h.handleError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

const pathAuditConfiguration = "/audit/configuration"

func resolveAuditConfigOps(path, method string) string {
	switch {
	case path == pathAuditConfiguration && method == http.MethodGet:
		return opDescribeAccountAuditConfiguration
	case path == pathAuditConfiguration && method == http.MethodPatch:
		return opUpdateAccountAuditConfiguration
	case path == pathAuditConfiguration && method == http.MethodDelete:
		return opDeleteAccountAuditConfiguration
	case path == "/audit/tasks" && method == http.MethodGet:
		return opListAuditTasks
	case path == "/audit/tasks" && method == http.MethodPost:
		return opStartOnDemandAuditTask
	case strings.HasPrefix(path, "/audit/tasks/") && method == http.MethodGet:
		return opDescribeAuditTask
	}

	return unknownOperation
}

func (h *Handler) handleDescribeAccountAuditConfiguration(c *echo.Context) error {
	cfg := h.Backend.DescribeAccountAuditConfiguration()

	return c.JSON(http.StatusOK, cfg)
}

func (h *Handler) handleUpdateAccountAuditConfiguration(c *echo.Context) error {
	var req struct {
		AuditCheckConfigurations map[string]*AuditCheckConfig `json:"auditCheckConfigurations,omitempty"`
		RoleARN                  string                       `json:"roleArn,omitempty"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}
	if err := h.Backend.UpdateAccountAuditConfiguration(req.RoleARN, req.AuditCheckConfigurations); err != nil {
		return respondErr(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleDeleteAccountAuditConfiguration(c *echo.Context) error {
	if err := h.Backend.DeleteAccountAuditConfiguration(); err != nil {
		return respondErr(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleStartOnDemandAuditTask(c *echo.Context) error {
	var req struct {
		TargetCheckNames []string `json:"targetCheckNames"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}
	taskID, err := h.Backend.StartOnDemandAuditTask(req.TargetCheckNames)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{keyTaskID: taskID})
}

func (h *Handler) handleDescribeAuditTask(c *echo.Context) error {
	taskID := strings.TrimPrefix(c.Request().URL.Path, "/audit/tasks/")
	task, err := h.Backend.DescribeAuditTask(taskID)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, task)
}

func (h *Handler) handleListAuditTasks(c *echo.Context) error {
	taskType := c.Request().URL.Query().Get("taskType")
	tasks := h.Backend.ListAuditTasks(taskType)
	summaries := make([]map[string]any, len(tasks))
	for i, t := range tasks {
		summaries[i] = map[string]any{
			keyTaskID:     t.TaskID,
			keyTaskStatus: t.TaskStatus,
			"taskType":    t.TaskType,
		}
	}

	return c.JSON(http.StatusOK, map[string]any{keyTasksField: summaries})
}

// resolveAuditSuppressionOps resolves the audit-suppression op family.
//
// Three of these were wrong against the real wire shape (iot@v1.77.4
// serializers.go), found by gopherstack-n1mb's route table:
// CreateAuditSuppression's real path is POST /audit/suppressions/create,
// not the bare /audit/suppressions; DescribeAuditSuppression and
// ListAuditSuppressions are both real POST (their filter fields are
// carried in a JSON body), not GET. The old bare-path/GET shapes are kept
// too as non-canonical routes wired for this package's own tests.
func resolveAuditSuppressionOps(path, method string) string {
	if op := resolveAuditSuppressionCRUDOps(path, method); op != unknownOperation {
		return op
	}

	return resolveAuditFindingOps(path, method)
}

func resolveAuditSuppressionCRUDOps(path, method string) string {
	switch {
	case path == "/audit/suppressions/create" && method == http.MethodPost:

		return opCreateAuditSuppression
	case path == "/audit/suppressions" && method == http.MethodPost:

		return opCreateAuditSuppression
	case path == "/audit/suppressions/describe" && (method == http.MethodPost || method == http.MethodGet):

		return opDescribeAuditSuppression
	case path == "/audit/suppressions/delete" && method == http.MethodPost:

		return opDeleteAuditSuppression
	case path == "/audit/suppressions/update" && method == http.MethodPatch:

		return opUpdateAuditSuppression
	case path == "/audit/suppressions/list" && (method == http.MethodPost || method == http.MethodGet):

		return opListAuditSuppressions
	}

	return unknownOperation
}

func resolveAuditFindingOps(path, method string) string {
	switch {
	case strings.HasPrefix(path, "/audit/findings/") && method == http.MethodGet:

		return opDescribeAuditFinding
	// ListAuditFindings is POST /audit/findings (its filter fields --
	// checkName/taskId/startTime/endTime/listSuppressedFindings/
	// resourceIdentifier -- are carried in a JSON body, not query params),
	// confirmed against aws-sdk-go-v2/service/iot@v1.77.4's serializers.go
	// http bindings. Previously matched on GET, which no real client ever
	// sends -- a genuine, previously-undiscovered routing bug that made this
	// op unreachable by a real SDK client. Fixed this pass; see PARITY.md.
	case path == "/audit/findings" && method == http.MethodPost:

		return opListAuditFindings
	}

	return unknownOperation
}

func (h *Handler) handleCreateAuditSuppression(c *echo.Context) error {
	var req struct {
		ResourceIdentifier   map[string]any `json:"resourceIdentifier"`
		CheckName            string         `json:"checkName"`
		Description          string         `json:"description"`
		SuppressIndefinitely bool           `json:"suppressIndefinitely"`
		ExpirationDate       float64        `json:"expirationDate"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}
	if err := h.Backend.CreateAuditSuppression(
		req.CheckName, req.ResourceIdentifier, req.Description, req.SuppressIndefinitely, req.ExpirationDate,
	); err != nil {
		return respondErr(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleDescribeAuditSuppression(c *echo.Context) error {
	var req struct {
		ResourceIdentifier map[string]any `json:"resourceIdentifier"`
		CheckName          string         `json:"checkName"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}
	s, err := h.Backend.DescribeAuditSuppression(req.CheckName, req.ResourceIdentifier)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, s)
}

func (h *Handler) handleUpdateAuditSuppression(c *echo.Context) error {
	var req struct {
		ResourceIdentifier   map[string]any `json:"resourceIdentifier"`
		CheckName            string         `json:"checkName"`
		Description          string         `json:"description"`
		SuppressIndefinitely bool           `json:"suppressIndefinitely"`
		ExpirationDate       float64        `json:"expirationDate"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}
	if err := h.Backend.UpdateAuditSuppression(
		req.CheckName, req.ResourceIdentifier, req.Description, req.SuppressIndefinitely, req.ExpirationDate,
	); err != nil {
		return respondErr(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleDeleteAuditSuppression(c *echo.Context) error {
	var req struct {
		ResourceIdentifier map[string]any `json:"resourceIdentifier"`
		CheckName          string         `json:"checkName"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}
	if err := h.Backend.DeleteAuditSuppression(req.CheckName, req.ResourceIdentifier); err != nil {
		// DeleteAuditSuppression's own deserializeOpError switch declares
		// no ResourceNotFoundException case.
		return respondAsInvalidRequest(c, err, ErrResourceNotFound)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleListAuditSuppressions(c *echo.Context) error {
	var req struct {
		ResourceIdentifier map[string]any `json:"resourceIdentifier"`
		// AscendingOrder is *bool, not bool: the real field "isn't provided"
		// (absent from the JSON body) vs. explicitly false are different
		// wire states -- "If parameter isn't provided, ascendingOrder=true"
		// (iot@v1.77.4 api_op_ListAuditSuppressions.go), so presence must be
		// distinguishable to apply that default correctly.
		AscendingOrder *bool  `json:"ascendingOrder"`
		CheckName      string `json:"checkName"`
		NextToken      string `json:"nextToken"`
		MaxResults     int    `json:"maxResults"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}

	items := h.Backend.ListAuditSuppressions()

	filtered := items[:0:0]

	for _, s := range items {
		if req.CheckName != "" && s.CheckName != req.CheckName {
			continue
		}

		if !matchAuditSuppressionResourceIdentifier(req.ResourceIdentifier, s.ResourceIdentifier) {
			continue
		}

		filtered = append(filtered, s)
	}

	ascending := req.AscendingOrder == nil || *req.AscendingOrder
	sort.Slice(filtered, func(i, j int) bool {
		if ascending {
			return filtered[i].ExpirationDate < filtered[j].ExpirationDate
		}

		return filtered[i].ExpirationDate > filtered[j].ExpirationDate
	})

	pageSize := req.MaxResults
	if pageSize <= 0 {
		pageSize = iotDefaultPageSize
	}

	start := 0

	if req.NextToken != "" {
		if n, err := strconv.Atoi(req.NextToken); err == nil && n > 0 {
			start = n
		}
	}

	page, nextToken := paginateMaps(filtered, pageSize, start)

	resp := map[string]any{"suppressions": page}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return c.JSON(http.StatusOK, resp)
}

// matchAuditSuppressionResourceIdentifier reports whether actual satisfies
// filter: every key set in filter must be present and equal in actual (same
// per-field discriminator semantics as this service's typed
// matchResourceIdentifier, but keyed dynamically since AuditSuppression
// stores ResourceIdentifier as an opaque map). A nil/empty filter always
// matches.
func matchAuditSuppressionResourceIdentifier(filter, actual map[string]any) bool {
	if len(filter) == 0 {
		return true
	}

	for k, v := range filter {
		if !reflect.DeepEqual(actual[k], v) {
			return false
		}
	}

	return true
}

func (h *Handler) handleDescribeAuditFinding(c *echo.Context) error {
	findingID := strings.TrimPrefix(c.Request().URL.Path, "/audit/findings/")
	f, err := h.Backend.DescribeAuditFinding(findingID)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"finding": f})
}

func (h *Handler) handleListAuditFindings(c *echo.Context) error {
	var req struct {
		ListSuppressedFindings *bool               `json:"listSuppressedFindings"`
		ResourceIdentifier     *ResourceIdentifier `json:"resourceIdentifier"`
		CheckName              string              `json:"checkName"`
		TaskID                 string              `json:"taskId"`
		StartTime              float64             `json:"startTime"`
		EndTime                float64             `json:"endTime"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}

	items := h.Backend.ListAuditFindings(ListAuditFindingsFilter{
		CheckName:              req.CheckName,
		TaskID:                 req.TaskID,
		StartTime:              req.StartTime,
		EndTime:                req.EndTime,
		ListSuppressedFindings: req.ListSuppressedFindings,
		ResourceIdentifier:     req.ResourceIdentifier,
	})

	return c.JSON(http.StatusOK, map[string]any{"findings": items})
}

func (h *Handler) handleDescribeEventConfigurations(c *echo.Context) error {
	cfg := h.Backend.DescribeEventConfigurations()

	return c.JSON(http.StatusOK, cfg)
}

func (h *Handler) handleUpdateEventConfigurations(c *echo.Context) error {
	var req struct {
		EventConfigurations map[string]*EventConfigEntry `json:"eventConfigurations"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}
	if err := h.Backend.UpdateEventConfigurations(req.EventConfigurations); err != nil {
		return respondErr(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func resolveScheduledAuditOps(path, method string) string {
	switch {
	case path == "/audit/scheduledaudits" && method == http.MethodGet:
		return opListScheduledAudits
	case strings.HasPrefix(path, "/audit/scheduledaudits/") && method == http.MethodPost:
		return opCreateScheduledAudit
	case strings.HasPrefix(path, "/audit/scheduledaudits/") && method == http.MethodGet:
		return opDescribeScheduledAudit
	case strings.HasPrefix(path, "/audit/scheduledaudits/") && method == http.MethodPatch:
		return opUpdateScheduledAudit
	case strings.HasPrefix(path, "/audit/scheduledaudits/") && method == http.MethodDelete:
		return opDeleteScheduledAudit
	}

	return unknownOperation
}

func resolveMitigationActionOps(path, method string) string {
	switch {
	case path == "/mitigationactions/actions" && method == http.MethodGet:
		return opListMitigationActions
	case strings.HasPrefix(path, "/mitigationactions/actions/") && method == http.MethodPost:
		return opCreateMitigationAction
	case strings.HasPrefix(path, "/mitigationactions/actions/") && method == http.MethodGet:
		return opDescribeMitigationAction
	case strings.HasPrefix(path, "/mitigationactions/actions/") && method == http.MethodPatch:
		return opUpdateMitigationAction
	case strings.HasPrefix(path, "/mitigationactions/actions/") && method == http.MethodDelete:
		return opDeleteMitigationAction
	}

	return unknownOperation
}

func (h *Handler) handleCreateScheduledAudit(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/audit/scheduledaudits/")
	var input CreateScheduledAuditInput
	if err := readBody(c, &input); err != nil {
		return err
	}
	input.ScheduledAuditName = name
	sa, err := h.Backend.CreateScheduledAudit(&input)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{keyScheduledAuditARN: sa.ScheduledAuditARN})
}

func (h *Handler) handleDescribeScheduledAudit(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/audit/scheduledaudits/")
	sa, err := h.Backend.DescribeScheduledAudit(name)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, sa)
}

func (h *Handler) handleListScheduledAudits(c *echo.Context) error {
	audits := h.Backend.ListScheduledAudits()
	summaries := make([]map[string]any, len(audits))
	for i, sa := range audits {
		summaries[i] = map[string]any{
			"scheduledAuditName": sa.ScheduledAuditName,
			keyScheduledAuditARN: sa.ScheduledAuditARN,
			"frequency":          sa.Frequency,
			"dayOfMonth":         sa.DayOfMonth,
			"dayOfWeek":          sa.DayOfWeek,
		}
	}

	return c.JSON(http.StatusOK, map[string]any{"scheduledAudits": summaries})
}

func (h *Handler) handleUpdateScheduledAudit(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/audit/scheduledaudits/")
	var req struct {
		Frequency        string   `json:"frequency"`
		DayOfMonth       string   `json:"dayOfMonth"`
		DayOfWeek        string   `json:"dayOfWeek"`
		TargetCheckNames []string `json:"targetCheckNames"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}
	sa, err := h.Backend.UpdateScheduledAudit(
		name,
		req.Frequency,
		req.DayOfMonth,
		req.DayOfWeek,
		req.TargetCheckNames,
	)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{keyScheduledAuditARN: sa.ScheduledAuditARN})
}

func (h *Handler) handleDeleteScheduledAudit(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/audit/scheduledaudits/")
	if err := h.Backend.DeleteScheduledAudit(name); err != nil {
		return respondErr(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleCreateMitigationAction(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/mitigationactions/actions/")
	var input CreateMitigationActionInput
	if err := readBody(c, &input); err != nil {
		return err
	}
	input.ActionName = name
	ma, err := h.Backend.CreateMitigationAction(&input)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyActionARN: ma.ActionARN,
		"actionId":   ma.ActionID,
	})
}

func (h *Handler) handleDescribeMitigationAction(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/mitigationactions/actions/")
	ma, err := h.Backend.DescribeMitigationAction(name)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, ma)
}

func (h *Handler) handleListMitigationActions(c *echo.Context) error {
	actions := h.Backend.ListMitigationActions()
	summaries := make([]map[string]any, len(actions))
	for i, ma := range actions {
		summaries[i] = map[string]any{
			"actionName": ma.ActionName,
			keyActionARN: ma.ActionARN,
		}
	}

	return c.JSON(http.StatusOK, map[string]any{"actionIdentifiers": summaries})
}

func (h *Handler) handleUpdateMitigationAction(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/mitigationactions/actions/")
	var req struct {
		ActionParams map[string]any `json:"actionParams"`
		RoleARN      string         `json:"roleArn"`
	}
	if err := readBody(c, &req); err != nil {
		return err
	}
	ma, err := h.Backend.UpdateMitigationAction(name, req.RoleARN, req.ActionParams)
	if err != nil {
		return respondErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyActionARN: ma.ActionARN,
		"actionId":   ma.ActionID,
	})
}

func (h *Handler) handleDeleteMitigationAction(c *echo.Context) error {
	name := strings.TrimPrefix(c.Request().URL.Path, "/mitigationactions/actions/")
	if err := h.Backend.DeleteMitigationAction(name); err != nil {
		// DeleteMitigationAction's own deserializeOpError switch declares
		// no ResourceNotFoundException case.
		return respondAsInvalidRequest(c, err, ErrResourceNotFound)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) dispatchAuditConfigOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opDescribeAccountAuditConfiguration:
		return true, h.handleDescribeAccountAuditConfiguration(c)
	case opUpdateAccountAuditConfiguration:
		return true, h.handleUpdateAccountAuditConfiguration(c)
	case opDeleteAccountAuditConfiguration:
		return true, h.handleDeleteAccountAuditConfiguration(c)
	case opStartOnDemandAuditTask:
		return true, h.handleStartOnDemandAuditTask(c)
	case opDescribeAuditTask:
		return true, h.handleDescribeAuditTask(c)
	case opListAuditTasks:
		return true, h.handleListAuditTasks(c)
	case opCancelAuditTask:
		return true, h.handleCancelAuditTask(c)
	}

	return false, nil
}

func (h *Handler) dispatchScheduledAuditOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opCreateScheduledAudit:
		return true, h.handleCreateScheduledAudit(c)
	case opDescribeScheduledAudit:
		return true, h.handleDescribeScheduledAudit(c)
	case opListScheduledAudits:
		return true, h.handleListScheduledAudits(c)
	case opUpdateScheduledAudit:
		return true, h.handleUpdateScheduledAudit(c)
	case opDeleteScheduledAudit:
		return true, h.handleDeleteScheduledAudit(c)
	}

	return false, nil
}

func (h *Handler) dispatchMitigationActionOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opCreateMitigationAction:
		return true, h.handleCreateMitigationAction(c)
	case opDescribeMitigationAction:
		return true, h.handleDescribeMitigationAction(c)
	case opListMitigationActions:
		return true, h.handleListMitigationActions(c)
	case opUpdateMitigationAction:
		return true, h.handleUpdateMitigationAction(c)
	case opDeleteMitigationAction:
		return true, h.handleDeleteMitigationAction(c)
	}

	return false, nil
}

func (h *Handler) dispatchAuditSuppressionOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opCreateAuditSuppression:
		return true, h.handleCreateAuditSuppression(c)
	case opDescribeAuditSuppression:
		return true, h.handleDescribeAuditSuppression(c)
	case opUpdateAuditSuppression:
		return true, h.handleUpdateAuditSuppression(c)
	case opDeleteAuditSuppression:
		return true, h.handleDeleteAuditSuppression(c)
	case opListAuditSuppressions:
		return true, h.handleListAuditSuppressions(c)
	case opDescribeAuditFinding:
		return true, h.handleDescribeAuditFinding(c)
	case opListAuditFindings:
		return true, h.handleListAuditFindings(c)
	}

	return false, nil
}

func (h *Handler) dispatchEventConfigOps(c *echo.Context, op string) (bool, error) {
	switch op {
	case opDescribeEventConfigurations:
		return true, h.handleDescribeEventConfigurations(c)
	case opUpdateEventConfigurations:
		return true, h.handleUpdateEventConfigurations(c)
	}

	return false, nil
}
