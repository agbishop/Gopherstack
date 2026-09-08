package bedrock

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
)

// extractARPOperation mirrors routeARP's dispatch order exactly (same
// predicates, same precedence) so ExtractOperation and Handler() never
// disagree on an AutomatedReasoningPolicy op name. Previously this only
// recognized the four POST-method ops (Create/Cancel/CreateTestCase/
// CreateVersion); every GET/PATCH/DELETE op in the family -- correctly
// dispatched by routeARP all along -- resolved to "Unknown" here, a
// misclassification found by gopherstack-n1mb's route table (Handler()
// itself was never broken, only this hand-duplicated extraction tree).
func extractARPOperation(path, method string) (string, bool) {
	if !strings.HasPrefix(path, automatedReasoningPrefix) {
		return "", false
	}

	if path == automatedReasoningPrefix {
		return extractARPRootOp(method)
	}

	if op, ok := extractARPBuildWorkflowOp(path, method); ok {
		return op, true
	}

	if op, ok := extractARPTestCaseOp(path, method); ok {
		return op, true
	}

	if op, ok := extractARPVersionExportOp(path, method); ok {
		return op, true
	}

	return extractARPSingleItemOp(path, method)
}

func extractARPRootOp(method string) (string, bool) {
	switch method {
	case http.MethodPost:
		return "CreateAutomatedReasoningPolicy", true
	case http.MethodGet:
		return "ListAutomatedReasoningPolicies", true
	}

	return "", false
}

// extractARPBuildWorkflowOp mirrors routeARPBuildWorkflow (sub-resource then core).
func extractARPBuildWorkflowOp(path, method string) (string, bool) {
	if op, ok := extractARPBuildWorkflowSubResourceOp(path, method); ok {
		return op, true
	}

	return extractARPBuildWorkflowCoreOp(path, method)
}

func extractARPBuildWorkflowSubResourceOp(path, method string) (string, bool) {
	switch {
	case isARPBuildWorkflowAnnotationsPath(path) && method == http.MethodGet:
		return "GetAutomatedReasoningPolicyAnnotations", true
	case isARPBuildWorkflowAnnotationsPath(path) && method == http.MethodPatch:
		return "UpdateAutomatedReasoningPolicyAnnotations", true
	case isARPBuildWorkflowScenariosPath(path) && method == http.MethodGet:
		return "GetAutomatedReasoningPolicyNextScenario", true
	case isARPBuildWorkflowTestCaseResultPath(path) && method == http.MethodGet:
		return "GetAutomatedReasoningPolicyTestResult", true
	case isARPBuildWorkflowTestResultsPath(path) && method == http.MethodGet:
		return "ListAutomatedReasoningPolicyTestResults", true
	case isARPBuildWorkflowTestWorkflowsPath(path) && method == http.MethodPost:
		return "StartAutomatedReasoningPolicyTestWorkflow", true
	}

	return "", false
}

func extractARPBuildWorkflowCoreOp(path, method string) (string, bool) {
	switch {
	case isARPBuildWorkflowCancelPath(path) && method == http.MethodPost:
		return "CancelAutomatedReasoningPolicyBuildWorkflow", true
	case isARPBuildWorkflowResultAssetsPath(path) && method == http.MethodGet:
		return "GetAutomatedReasoningPolicyBuildWorkflowResultAssets", true
	case isARPBuildWorkflowStartPath(path) && method == http.MethodPost:
		return "StartAutomatedReasoningPolicyBuildWorkflow", true
	case isARPBuildWorkflowSubPath(path) && method == http.MethodGet:
		return "GetAutomatedReasoningPolicyBuildWorkflow", true
	case isARPBuildWorkflowSubPath(path) && method == http.MethodDelete:
		return "DeleteAutomatedReasoningPolicyBuildWorkflow", true
	case isARPBuildWorkflowsPath(path) && method == http.MethodGet:
		return "ListAutomatedReasoningPolicyBuildWorkflows", true
	}

	return "", false
}

func extractARPTestCaseOp(path, method string) (string, bool) {
	switch {
	case isARPTestCasesPath(path) && method == http.MethodPost:
		return "CreateAutomatedReasoningPolicyTestCase", true
	case isARPTestCasesPath(path) && method == http.MethodGet:
		return "ListAutomatedReasoningPolicyTestCases", true
	case isARPTestCaseSubPath(path) && method == http.MethodGet:
		return "GetAutomatedReasoningPolicyTestCase", true
	case isARPTestCaseSubPath(path) && method == http.MethodPatch:
		return "UpdateAutomatedReasoningPolicyTestCase", true
	case isARPTestCaseSubPath(path) && method == http.MethodDelete:
		return "DeleteAutomatedReasoningPolicyTestCase", true
	}

	return "", false
}

func extractARPVersionExportOp(path, method string) (string, bool) {
	switch {
	case isARPVersionsPath(path) && method == http.MethodPost:
		return "CreateAutomatedReasoningPolicyVersion", true
	case isARPExportPath(path) && method == http.MethodGet:
		return "ExportAutomatedReasoningPolicyVersion", true
	}

	return "", false
}

func extractARPSingleItemOp(path, method string) (string, bool) {
	if !strings.HasPrefix(path, automatedReasoningPrefix+"/") {
		return "", false
	}

	switch method {
	case http.MethodGet:
		return "GetAutomatedReasoningPolicy", true
	case http.MethodPatch:
		return "UpdateAutomatedReasoningPolicy", true
	case http.MethodDelete:
		return "DeleteAutomatedReasoningPolicy", true
	}

	return "", false
}

// isARPBuildWorkflowCancelPath matches /automated-reasoning-policies/{arn}/build-workflows/{id}/cancel.
func isARPBuildWorkflowCancelPath(path string) bool {
	rest, ok := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	if !ok {
		return false
	}

	return strings.Contains(rest, "/build-workflows/") && strings.HasSuffix(rest, "/cancel")
}

// isARPTestCasesPath matches /automated-reasoning-policies/{arn}/test-cases.
func isARPTestCasesPath(path string) bool {
	rest, ok := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	if !ok {
		return false
	}

	return strings.HasSuffix(rest, "/test-cases")
}

// isARPVersionsPath matches /automated-reasoning-policies/{arn}/versions.
func isARPVersionsPath(path string) bool {
	rest, ok := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	if !ok {
		return false
	}

	return strings.HasSuffix(rest, "/versions")
}

func (h *Handler) routeARP(c *echo.Context, path, method string, body []byte) (bool, error) {
	if !strings.HasPrefix(path, automatedReasoningPrefix) {
		return false, nil
	}

	if path == automatedReasoningPrefix {
		return h.routeARPRoot(c, method, body)
	}

	if ok, err := h.routeARPBuildWorkflow(c, path, method, body); ok {
		return true, err
	}

	if ok, err := h.routeARPTestCase(c, path, method, body); ok {
		return true, err
	}

	if ok, err := h.routeARPVersionExport(c, path, method, body); ok {
		return true, err
	}

	return h.routeARPSingleItem(c, path, method, body)
}

func (h *Handler) routeARPRoot(c *echo.Context, method string, body []byte) (bool, error) {
	switch method {
	case http.MethodPost:
		return true, h.handleCreateAutomatedReasoningPolicy(c, body)
	case http.MethodGet:
		return true, h.handleListAutomatedReasoningPolicies(c)
	}

	return false, nil
}

func (h *Handler) routeARPBuildWorkflow(c *echo.Context, path, method string, body []byte) (bool, error) {
	if ok, err := h.routeARPBuildWorkflowSubResource(c, path, method, body); ok {
		return true, err
	}

	return h.routeARPBuildWorkflowCore(c, path, method, body)
}

// routeARPBuildWorkflowSubResource handles the build-workflow-scoped sub-resources
// (annotations, scenarios, test-results, test-workflows) that real AWS nests under
// .../build-workflows/{buildWorkflowId}/... (bedrock@v1.66.4 serializers.go:3874,
// :4122, :4282, :5937, :8117).
func (h *Handler) routeARPBuildWorkflowSubResource(
	c *echo.Context,
	path, method string,
	body []byte,
) (bool, error) {
	switch {
	case isARPBuildWorkflowAnnotationsPath(path) && method == http.MethodGet:
		return true, h.handleGetARPAnnotations(c, path)
	// UpdateAutomatedReasoningPolicyAnnotations uses PATCH in the real SDK, not PUT.
	case isARPBuildWorkflowAnnotationsPath(path) && method == http.MethodPatch:
		return true, h.handleUpdateARPAnnotations(c, path, body)
	case isARPBuildWorkflowScenariosPath(path) && method == http.MethodGet:
		return true, h.handleGetARPNextScenario(c, path)
	case isARPBuildWorkflowTestCaseResultPath(path) && method == http.MethodGet:
		return true, h.handleGetARPTestResult(c, path)
	case isARPBuildWorkflowTestResultsPath(path) && method == http.MethodGet:
		return true, h.handleListARPTestResults(c, path)
	case isARPBuildWorkflowTestWorkflowsPath(path) && method == http.MethodPost:
		return true, h.handleStartARPTestWorkflow(c, path, body)
	}

	return false, nil
}

func (h *Handler) routeARPBuildWorkflowCore(c *echo.Context, path, method string, body []byte) (bool, error) {
	switch {
	case isARPBuildWorkflowCancelPath(path) && method == http.MethodPost:
		return true, h.handleCancelAutomatedReasoningPolicyBuildWorkflow(c, path)
	case isARPBuildWorkflowResultAssetsPath(path) && method == http.MethodGet:
		return true, h.handleGetARPBuildWorkflowResultAssets(c, path)
	// StartAutomatedReasoningPolicyBuildWorkflow's real path is
	// .../build-workflows/{buildWorkflowType}/start (bedrock@v1.66.4
	// serializers.go:8008), not bare .../build-workflows -- must be checked
	// before isARPBuildWorkflowsPath below, whose POST case a real client's
	// Start request never actually reaches.
	case isARPBuildWorkflowStartPath(path) && method == http.MethodPost:
		return true, h.handleStartARPBuildWorkflow(c, path, body)
	case isARPBuildWorkflowSubPath(path) && method == http.MethodGet:
		return true, h.handleGetARPBuildWorkflow(c, path)
	case isARPBuildWorkflowSubPath(path) && method == http.MethodDelete:
		return true, h.handleDeleteARPBuildWorkflow(c, path)
	case isARPBuildWorkflowsPath(path) && method == http.MethodGet:
		return true, h.handleListARPBuildWorkflows(c, path)
	}

	return false, nil
}

func (h *Handler) routeARPTestCase(c *echo.Context, path, method string, body []byte) (bool, error) {
	if ok, err := h.routeARPTestCaseCreate(c, path, method); ok {
		return true, err
	}

	return h.routeARPTestCaseItem(c, path, method, body)
}

func (h *Handler) routeARPTestCaseCreate(c *echo.Context, path, method string) (bool, error) {
	switch {
	case isARPTestCasesPath(path) && method == http.MethodPost:
		return true, h.handleCreateAutomatedReasoningPolicyTestCase(c, path)
	case isARPTestCasesPath(path) && method == http.MethodGet:
		return true, h.handleListARPTestCases(c, path)
	}

	return false, nil
}

// routeARPTestCaseItem handles the plain policy-scoped test-case CRUD paths
// (.../test-cases/{testCaseId}, bedrock@v1.66.4 serializers.go:2595, :4202,
// :8903) -- unaffected by the build-workflow rescoping above.
func (h *Handler) routeARPTestCaseItem(c *echo.Context, path, method string, body []byte) (bool, error) {
	switch {
	case isARPTestCaseSubPath(path) && method == http.MethodGet:
		return true, h.handleGetARPTestCase(c, path)
	// UpdateAutomatedReasoningPolicyTestCase uses PATCH in the real SDK, not PUT.
	case isARPTestCaseSubPath(path) && method == http.MethodPatch:
		return true, h.handleUpdateARPTestCase(c, path, body)
	case isARPTestCaseSubPath(path) && method == http.MethodDelete:
		return true, h.handleDeleteARPTestCase(c, path)
	}

	return false, nil
}

func (h *Handler) routeARPVersionExport(
	c *echo.Context,
	path, method string,
	body []byte,
) (bool, error) {
	switch {
	case isARPVersionsPath(path) && method == http.MethodPost:
		return true, h.handleCreateAutomatedReasoningPolicyVersion(c, path, body)
	// ExportAutomatedReasoningPolicyVersion uses GET in the real SDK, not POST
	// (bedrock@v1.66.4 serializers.go:3603).
	case isARPExportPath(path) && method == http.MethodGet:
		return true, h.handleExportARPVersion(c, path)
	}

	return false, nil
}

func (h *Handler) routeARPSingleItem(
	c *echo.Context,
	path, method string,
	body []byte,
) (bool, error) {
	if !strings.HasPrefix(path, automatedReasoningPrefix+"/") {
		return false, nil
	}

	policyARN := decodePath(strings.TrimPrefix(path, automatedReasoningPrefix+"/"))

	switch method {
	case http.MethodGet:
		return true, h.handleGetAutomatedReasoningPolicy(c, policyARN)
	// UpdateAutomatedReasoningPolicy uses PATCH in the real SDK, not PUT.
	case http.MethodPatch:
		return true, h.handleUpdateAutomatedReasoningPolicy(c, policyARN, body)
	case http.MethodDelete:
		return true, h.handleDeleteAutomatedReasoningPolicy(c, policyARN)
	}

	return false, nil
}

type createAutomatedReasoningPolicyInput struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Tags        []Tag  `json:"tags,omitempty"`
}

type createAutomatedReasoningPolicyOutput struct {
	CreatedAt      isoTime `json:"createdAt"`
	UpdatedAt      isoTime `json:"updatedAt"`
	PolicyArn      string  `json:"policyArn"`
	Name           string  `json:"name"`
	Status         string  `json:"status"`
	DefinitionHash string  `json:"definitionHash,omitempty"`
	Version        string  `json:"version,omitempty"`
}

func (h *Handler) handleCreateAutomatedReasoningPolicy(c *echo.Context, body []byte) error {
	in, err := parseBody[createAutomatedReasoningPolicyInput](body)
	if err != nil {
		return c.JSON(
			http.StatusBadRequest,
			errorResponse("ValidationException", "invalid request body"),
		)
	}

	policy, opErr := h.Backend.CreateAutomatedReasoningPolicy(in.Name, in.Description, in.Tags)
	if opErr != nil {
		return h.writeError(c, opErr)
	}

	return c.JSON(http.StatusCreated, createAutomatedReasoningPolicyOutput{
		PolicyArn:      policy.PolicyArn,
		Name:           policy.Name,
		Status:         policy.Status,
		CreatedAt:      isoTime{policy.CreatedAt},
		UpdatedAt:      isoTime{policy.UpdatedAt},
		DefinitionHash: policy.DefinitionHash,
		Version:        policy.Version,
	})
}

// handleCancelAutomatedReasoningPolicyBuildWorkflow cancels a build workflow.
// Path: /automated-reasoning-policies/{policyArn}/build-workflows/{buildWorkflowId}/cancel.
func (h *Handler) handleCancelAutomatedReasoningPolicyBuildWorkflow(
	c *echo.Context,
	path string,
) error {
	policyARN, workflowID := extractARPBuildWorkflowIDs(path)

	if opErr := h.Backend.CancelAutomatedReasoningPolicyBuildWorkflow(policyARN, workflowID); opErr != nil {
		return h.writeError(c, opErr)
	}

	return c.NoContent(http.StatusOK)
}

// extractARPBuildWorkflowIDs parses policyArn and workflowId from the cancel path.
func extractARPBuildWorkflowIDs(path string) (string, string) {
	rest, _ := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	// rest = {policyArn}/build-workflows/{buildWorkflowId}/cancel
	beforeCancel := strings.TrimSuffix(rest, "/cancel")
	idx := strings.LastIndex(beforeCancel, "/build-workflows/")

	if idx < 0 {
		return "", ""
	}

	policyARN := decodePath(beforeCancel[:idx])
	workflowID := beforeCancel[idx+len("/build-workflows/"):]

	return policyARN, workflowID
}

// handleCreateAutomatedReasoningPolicyTestCase creates a test case.
// Path: /automated-reasoning-policies/{policyArn}/test-cases.
func (h *Handler) handleCreateAutomatedReasoningPolicyTestCase(c *echo.Context, path string) error {
	rest, _ := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	policyARN := decodePath(strings.TrimSuffix(rest, "/test-cases"))

	tc, opErr := h.Backend.CreateAutomatedReasoningPolicyTestCase(policyARN)
	if opErr != nil {
		return h.writeError(c, opErr)
	}

	return c.JSON(http.StatusCreated, map[string]string{
		"policyArn":   tc.PolicyArn,
		keyTestCaseID: tc.TestCaseID,
	})
}

type createAutomatedReasoningPolicyVersionInput struct {
	LastUpdatedDefinitionHash string `json:"lastUpdatedDefinitionHash"`
	Tags                      []Tag  `json:"tags,omitempty"`
}

// handleCreateAutomatedReasoningPolicyVersion creates a policy version.
// Path: /automated-reasoning-policies/{policyArn}/versions.
func (h *Handler) handleCreateAutomatedReasoningPolicyVersion(
	c *echo.Context,
	path string,
	body []byte,
) error {
	in, err := parseBody[createAutomatedReasoningPolicyVersionInput](body)
	if err != nil {
		return c.JSON(
			http.StatusBadRequest,
			errorResponse("ValidationException", "invalid request body"),
		)
	}

	rest, _ := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	policyARN := decodePath(strings.TrimSuffix(rest, "/versions"))

	version, opErr := h.Backend.CreateAutomatedReasoningPolicyVersion(
		policyARN,
		in.LastUpdatedDefinitionHash,
		in.Tags,
	)
	if opErr != nil {
		return h.writeError(c, opErr)
	}

	return c.JSON(http.StatusCreated, map[string]any{
		keyPolicyArn:      version.PolicyArn,
		keyName:           version.Name,
		keyDefinitionHash: version.DefinitionHash,
		keyVersion:        version.Version,
		"createdAt":       isoTime{version.CreatedAt},
	})
}

// isARPBuildWorkflowsPath matches /automated-reasoning-policies/{arn}/build-workflows (no trailing segment).
func isARPBuildWorkflowsPath(path string) bool {
	rest, ok := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	if !ok {
		return false
	}

	return strings.HasSuffix(rest, "/build-workflows")
}

// isARPBuildWorkflowStartPath matches
// /automated-reasoning-policies/{arn}/build-workflows/{buildWorkflowType}/start
// (bedrock@v1.66.4 serializers.go:8008).
func isARPBuildWorkflowStartPath(path string) bool {
	rest, ok := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	if !ok {
		return false
	}

	return strings.Contains(rest, "/build-workflows/") && strings.HasSuffix(rest, "/start")
}

// extractARPBuildWorkflowStart extracts policyARN and buildWorkflowType from
// the Start path.
func extractARPBuildWorkflowStart(path string) (string, string) {
	rest, _ := strings.CutPrefix(path, automatedReasoningPrefix+"/")

	before, after, ok := strings.Cut(rest, "/build-workflows/")
	if !ok {
		return "", ""
	}

	return decodePath(before), strings.TrimSuffix(after, "/start")
}

// isARPBuildWorkflowSubPath matches /automated-reasoning-policies/{arn}/build-workflows/{id}
// but NOT sub-paths like /cancel or /result-assets.
func isARPBuildWorkflowSubPath(path string) bool {
	rest, ok := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	if !ok {
		return false
	}

	_, after, ok := strings.Cut(rest, "/build-workflows/")
	if !ok {
		return false
	}

	segment := after

	return !strings.Contains(segment, "/")
}

// isARPBuildWorkflowResultAssetsPath matches /automated-reasoning-policies/{arn}/build-workflows/{id}/result-assets.
func isARPBuildWorkflowResultAssetsPath(path string) bool {
	rest, ok := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	if !ok {
		return false
	}

	return strings.Contains(rest, "/build-workflows/") && strings.HasSuffix(rest, "/result-assets")
}

// isARPTestCaseSubPath matches /automated-reasoning-policies/{arn}/test-cases/{id}
// but NOT /test-cases or further nesting.
func isARPTestCaseSubPath(path string) bool {
	rest, ok := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	if !ok {
		return false
	}

	_, after, ok := strings.Cut(rest, "/test-cases/")
	if !ok {
		return false
	}

	return !strings.Contains(after, "/")
}

// isARPBuildWorkflowAnnotationsPath matches
// /automated-reasoning-policies/{arn}/build-workflows/{id}/annotations.
func isARPBuildWorkflowAnnotationsPath(path string) bool {
	rest, ok := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	if !ok {
		return false
	}

	return strings.Contains(rest, "/build-workflows/") && strings.HasSuffix(rest, "/annotations")
}

// isARPBuildWorkflowScenariosPath matches
// /automated-reasoning-policies/{arn}/build-workflows/{id}/scenarios.
func isARPBuildWorkflowScenariosPath(path string) bool {
	rest, ok := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	if !ok {
		return false
	}

	return strings.Contains(rest, "/build-workflows/") && strings.HasSuffix(rest, "/scenarios")
}

// isARPBuildWorkflowTestCaseResultPath matches
// /automated-reasoning-policies/{arn}/build-workflows/{id}/test-cases/{testCaseId}/test-results.
func isARPBuildWorkflowTestCaseResultPath(path string) bool {
	rest, ok := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	if !ok {
		return false
	}

	return strings.Contains(rest, "/build-workflows/") &&
		strings.Contains(rest, "/test-cases/") &&
		strings.HasSuffix(rest, "/test-results")
}

// isARPBuildWorkflowTestResultsPath matches
// /automated-reasoning-policies/{arn}/build-workflows/{id}/test-results (the list,
// not the per-test-case isARPBuildWorkflowTestCaseResultPath above).
func isARPBuildWorkflowTestResultsPath(path string) bool {
	rest, ok := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	if !ok {
		return false
	}

	return strings.Contains(rest, "/build-workflows/") &&
		strings.HasSuffix(rest, "/test-results") &&
		!strings.Contains(rest, "/test-cases/")
}

// isARPBuildWorkflowTestWorkflowsPath matches
// /automated-reasoning-policies/{arn}/build-workflows/{id}/test-workflows.
func isARPBuildWorkflowTestWorkflowsPath(path string) bool {
	rest, ok := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	if !ok {
		return false
	}

	return strings.Contains(rest, "/build-workflows/") && strings.HasSuffix(rest, "/test-workflows")
}

// isARPExportPath matches /automated-reasoning-policies/{policyArn}/export; policyArn
// may itself embed a version segment (bedrock@v1.66.4 serializers.go:3603 -- there is
// no separate {version} path segment).
func isARPExportPath(path string) bool {
	rest, ok := strings.CutPrefix(path, automatedReasoningPrefix+"/")
	if !ok {
		return false
	}

	return strings.HasSuffix(rest, "/export")
}

// extractARPPolicyARN extracts the policyARN from a path that has a known suffix.
func extractARPPolicyARN(path, suffix string) string {
	rest, _ := strings.CutPrefix(path, automatedReasoningPrefix+"/")

	return decodePath(strings.TrimSuffix(rest, suffix))
}

// extractARPWorkflowIDs extracts policyARN and workflowID from a build-workflow path.
func extractARPWorkflowIDs(path string) (string, string) {
	rest, _ := strings.CutPrefix(path, automatedReasoningPrefix+"/")

	before, after, ok := strings.Cut(rest, "/build-workflows/")
	if !ok {
		return "", ""
	}

	policyARN := decodePath(before)
	// Strip any trailing sub-path like /result-assets, /cancel.
	workflowID, _, _ := strings.Cut(after, "/")

	return policyARN, workflowID
}

// extractARPTestCaseIDs extracts policyARN and testCaseID from a test-case path.
func extractARPTestCaseIDs(path string) (string, string) {
	rest, _ := strings.CutPrefix(path, automatedReasoningPrefix+"/")

	before, after, ok := strings.Cut(rest, "/test-cases/")
	if !ok {
		return "", ""
	}

	policyARN := decodePath(before)
	testCaseID, _, _ := strings.Cut(after, "/")

	return policyARN, testCaseID
}

// extractARPBuildWorkflowTestCaseIDs extracts policyARN, buildWorkflowID, and
// testCaseID from a .../build-workflows/{id}/test-cases/{testCaseId}/test-results path.
func extractARPBuildWorkflowTestCaseIDs(path string) (string, string, string) {
	rest, _ := strings.CutPrefix(path, automatedReasoningPrefix+"/")

	before, after, ok := strings.Cut(rest, "/build-workflows/")
	if !ok {
		return "", "", ""
	}

	wfID, after2, ok := strings.Cut(after, "/test-cases/")
	if !ok {
		return "", "", ""
	}

	return decodePath(before), wfID, strings.TrimSuffix(after2, "/test-results")
}

// extractARPExportPolicyArn extracts the (possibly versioned) ARN from an export path.
func extractARPExportPolicyArn(path string) string {
	rest, _ := strings.CutPrefix(path, automatedReasoningPrefix+"/")

	return decodePath(strings.TrimSuffix(rest, "/export"))
}

// policyIDFromARN extracts the policy ID path segment from an Automated
// Reasoning policy ARN, draft or versioned
// (".../automated-reasoning-policy/{id}[/version/{n}]"). This is the same ID
// CreateAutomatedReasoningPolicy embedded when it built the ARN, not a
// fabricated value.
func policyIDFromARN(policyARN string) string {
	const marker = "automated-reasoning-policy/"

	_, rest, ok := strings.Cut(policyARN, marker)
	if !ok {
		return ""
	}

	id, _, _ := strings.Cut(rest, "/")

	return id
}

func (h *Handler) handleGetAutomatedReasoningPolicy(c *echo.Context, policyARN string) error {
	policy, err := h.Backend.GetAutomatedReasoningPolicy(policyARN)
	if err != nil {
		return h.writeError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyPolicyArn:      policy.PolicyArn,
		"policyId":        policyIDFromARN(policy.PolicyArn),
		keyName:           policy.Name,
		"description":     policy.Description,
		keyDefinitionHash: policy.DefinitionHash,
		keyVersion:        policy.Version,
		keyCreatedAt:      isoTime{policy.CreatedAt},
		keyUpdatedAt:      isoTime{policy.UpdatedAt},
	})
}

func (h *Handler) handleListAutomatedReasoningPolicies(c *echo.Context) error {
	policies := h.Backend.ListAutomatedReasoningPolicies()
	summaries := make([]map[string]any, 0, len(policies))

	for _, p := range policies {
		summaries = append(summaries, map[string]any{
			keyPolicyArn: p.PolicyArn,
			keyName:      p.Name,
			keyStatus:    p.Status,
			keyCreatedAt: isoTime{p.CreatedAt},
			keyUpdatedAt: isoTime{p.UpdatedAt},
		})
	}

	// Real key is automatedReasoningPolicySummaries (bedrock@v1.66.4
	// deserializers.go, awsRestjson1_deserializeOpDocumentListAutomatedReasoningPoliciesOutput).
	return c.JSON(http.StatusOK, map[string]any{"automatedReasoningPolicySummaries": summaries})
}

type updateARPInput struct {
	Description      string          `json:"description,omitempty"`
	Name             string          `json:"name,omitempty"`
	PolicyDefinition json.RawMessage `json:"policyDefinition"`
}

func (h *Handler) handleUpdateAutomatedReasoningPolicy(c *echo.Context, policyARN string, body []byte) error {
	in, err := parseBody[updateARPInput](body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse("ValidationException", "invalid request body"))
	}

	if len(in.PolicyDefinition) == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse("ValidationException", "policyDefinition is required"))
	}

	policy, opErr := h.Backend.UpdateAutomatedReasoningPolicy(policyARN, in.Name, in.Description, in.PolicyDefinition)
	if opErr != nil {
		return h.writeError(c, opErr)
	}

	// Real UpdateAutomatedReasoningPolicyOutput has definitionHash/name/policyArn/
	// updatedAt -- no status (bedrock@v1.66.4 deserializers.go:17590-17650).
	return c.JSON(http.StatusOK, map[string]any{
		keyPolicyArn:      policy.PolicyArn,
		keyName:           policy.Name,
		keyDefinitionHash: policy.DefinitionHash,
		keyUpdatedAt:      isoTime{policy.UpdatedAt},
	})
}

func (h *Handler) handleDeleteAutomatedReasoningPolicy(c *echo.Context, policyARN string) error {
	force, _ := strconv.ParseBool(c.QueryParam("force"))

	if err := h.Backend.DeleteAutomatedReasoningPolicy(policyARN, force); err != nil {
		return h.writeError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// handleStartARPBuildWorkflow starts a build workflow. Path:
// /automated-reasoning-policies/{policyArn}/build-workflows/{buildWorkflowType}/start;
// body is the entire required sourceContent object, not a wrapper (bedrock@v1.66.4
// serializers.go:8008-8058). sourceContent's own nested union is not
// interpreted -- stored verbatim on the workflow (AutomatedReasoningPolicyBuildWorkflow.
// SourceContent) as real, inert backing state, never fabricated.
func (h *Handler) handleStartARPBuildWorkflow(c *echo.Context, path string, body []byte) error {
	policyARN, buildWorkflowType := extractARPBuildWorkflowStart(path)

	if !json.Valid(body) {
		return c.JSON(http.StatusBadRequest, errorResponse("ValidationException", "invalid request body"))
	}

	wf, err := h.Backend.StartAutomatedReasoningPolicyBuildWorkflow(
		policyARN, buildWorkflowType, json.RawMessage(body),
	)
	if err != nil {
		return h.writeError(c, err)
	}

	// Real StartAutomatedReasoningPolicyBuildWorkflowOutput has only
	// buildWorkflowId/policyArn -- no status (bedrock@v1.66.4
	// api_op_StartAutomatedReasoningPolicyBuildWorkflow.go:59-76).
	return c.JSON(http.StatusCreated, map[string]any{
		keyBuildWorkflowID: wf.BuildWorkflowID,
		keyPolicyArn:       wf.PolicyArn,
	})
}

func (h *Handler) handleGetARPBuildWorkflow(c *echo.Context, path string) error {
	policyARN, workflowID := extractARPWorkflowIDs(path)

	wf, err := h.Backend.GetAutomatedReasoningPolicyBuildWorkflow(policyARN, workflowID)
	if err != nil {
		return h.writeError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyBuildWorkflowID:  wf.BuildWorkflowID,
		keyPolicyArn:        wf.PolicyArn,
		keyStatus:           wf.Status,
		"buildWorkflowType": wf.BuildWorkflowType,
		keyCreatedAt:        isoTime{wf.CreatedAt},
		keyUpdatedAt:        isoTime{wf.UpdatedAt},
	})
}

func (h *Handler) handleListARPBuildWorkflows(c *echo.Context, path string) error {
	policyARN := extractARPPolicyARN(path, "/build-workflows")
	workflows := h.Backend.ListAutomatedReasoningPolicyBuildWorkflows(policyARN)
	summaries := make([]map[string]any, 0, len(workflows))

	for _, wf := range workflows {
		summaries = append(summaries, map[string]any{
			keyBuildWorkflowID:  wf.BuildWorkflowID,
			keyPolicyArn:        wf.PolicyArn,
			keyStatus:           wf.Status,
			"buildWorkflowType": wf.BuildWorkflowType,
			keyCreatedAt:        isoTime{wf.CreatedAt},
			keyUpdatedAt:        isoTime{wf.UpdatedAt},
		})
	}

	// Real key is automatedReasoningPolicyBuildWorkflowSummaries (bedrock@v1.66.4
	// deserializers.go, awsRestjson1_deserializeOpDocumentListAutomatedReasoningPolicyBuildWorkflowsOutput).
	return c.JSON(http.StatusOK, map[string]any{"automatedReasoningPolicyBuildWorkflowSummaries": summaries})
}

func (h *Handler) handleDeleteARPBuildWorkflow(c *echo.Context, path string) error {
	policyARN, workflowID := extractARPWorkflowIDs(path)

	if err := h.Backend.DeleteAutomatedReasoningPolicyBuildWorkflow(policyARN, workflowID); err != nil {
		return h.writeError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// handleGetARPBuildWorkflowResultAssets deliberately does not read the
// assetType/assetId query params (bedrock@v1.66.4 serializers.go:4068-4074).
// See GetAutomatedReasoningPolicyBuildWorkflowResultAssets's doc comment for
// why: the backend has no result-asset content to filter, so wiring the
// param would accept it without it ever changing the (always-empty) output --
// unverifiable surface, not a real fix.
func (h *Handler) handleGetARPBuildWorkflowResultAssets(c *echo.Context, path string) error {
	policyARN, workflowID := extractARPWorkflowIDs(path)

	result, err := h.Backend.GetAutomatedReasoningPolicyBuildWorkflowResultAssets(policyARN, workflowID)
	if err != nil {
		return h.writeError(c, err)
	}

	return c.JSON(http.StatusOK, result)
}

// arpTestCaseToMap builds the nested AutomatedReasoningPolicyTestCase shape
// (bedrock@v1.66.4 types.go:2016-2051: createdAt/guardContent/testCaseId/
// updatedAt required, the rest optional).
func arpTestCaseToMap(tc *AutomatedReasoningPolicyTestCase) map[string]any {
	return map[string]any{
		keyTestCaseID:                      tc.TestCaseID,
		keyPolicyArn:                       tc.PolicyArn,
		keyCreatedAt:                       isoTime{tc.CreatedAt},
		keyUpdatedAt:                       isoTime{tc.UpdatedAt},
		"guardContent":                     tc.GuardContent,
		"queryContent":                     tc.QueryContent,
		"expectedAggregatedFindingsResult": tc.ExpectedAggregatedFindingsResult,
		"confidenceThreshold":              tc.ConfidenceThreshold,
	}
}

// handleGetARPTestCase wraps the test case under the required "testCase" key
// (bedrock@v1.66.4 deserializers.go:7223-7233: GetAutomatedReasoningPolicyTestCaseOutput
// is policyArn + testCase, not the test case's own fields inlined at the top
// level) -- previously returned arpTestCaseToMap's flat shape directly, so a
// real client's required TestCase decoded nil.
func (h *Handler) handleGetARPTestCase(c *echo.Context, path string) error {
	policyARN, testCaseID := extractARPTestCaseIDs(path)

	tc, err := h.Backend.GetAutomatedReasoningPolicyTestCase(policyARN, testCaseID)
	if err != nil {
		return h.writeError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyPolicyArn: policyARN,
		"testCase":   arpTestCaseToMap(tc),
	})
}

func (h *Handler) handleListARPTestCases(c *echo.Context, path string) error {
	policyARN := extractARPPolicyARN(path, "/test-cases")
	cases := h.Backend.ListAutomatedReasoningPolicyTestCases(policyARN)
	summaries := make([]map[string]any, 0, len(cases))

	for _, tc := range cases {
		summaries = append(summaries, arpTestCaseToMap(tc))
	}

	return c.JSON(http.StatusOK, map[string]any{"testCases": summaries})
}

type updateARPTestCaseInput struct {
	ConfidenceThreshold              *float64 `json:"confidenceThreshold,omitempty"`
	GuardContent                     string   `json:"guardContent"`
	QueryContent                     string   `json:"queryContent,omitempty"`
	ExpectedAggregatedFindingsResult string   `json:"expectedAggregatedFindingsResult"`
	LastUpdatedAt                    string   `json:"lastUpdatedAt,omitempty"`
	ClientRequestToken               string   `json:"clientRequestToken,omitempty"`
}

func (h *Handler) handleUpdateARPTestCase(c *echo.Context, path string, body []byte) error {
	policyARN, testCaseID := extractARPTestCaseIDs(path)

	in, parseErr := parseBody[updateARPTestCaseInput](body)
	if parseErr != nil {
		return c.JSON(http.StatusBadRequest, errorResponse("ValidationException", "invalid request body"))
	}

	tc, err := h.Backend.UpdateAutomatedReasoningPolicyTestCase(
		policyARN, testCaseID, in.GuardContent, in.QueryContent,
		in.ExpectedAggregatedFindingsResult, in.ConfidenceThreshold,
	)
	if err != nil {
		return h.writeError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyTestCaseID: tc.TestCaseID,
		keyPolicyArn:  tc.PolicyArn,
	})
}

func (h *Handler) handleDeleteARPTestCase(c *echo.Context, path string) error {
	policyARN, testCaseID := extractARPTestCaseIDs(path)

	if err := h.Backend.DeleteAutomatedReasoningPolicyTestCase(policyARN, testCaseID); err != nil {
		return h.writeError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

type startARPTestWorkflowInput struct {
	ClientRequestToken string   `json:"clientRequestToken,omitempty"`
	TestCaseIDs        []string `json:"testCaseIds,omitempty"`
}

// handleStartARPTestWorkflow starts a test workflow for a build workflow, optionally
// scoped to a subset of test cases (bedrock@v1.66.4 serializers.go:8117 --
// .../build-workflows/{id}/test-workflows takes testCaseIds in the body, not a
// single test case in the path).
func (h *Handler) handleStartARPTestWorkflow(c *echo.Context, path string, body []byte) error {
	policyARN, workflowID := extractARPWorkflowIDs(path)

	in, err := parseBody[startARPTestWorkflowInput](body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse("ValidationException", "invalid request body"))
	}

	result, opErr := h.Backend.StartAutomatedReasoningPolicyTestWorkflow(policyARN, workflowID, in.TestCaseIDs)
	if opErr != nil {
		return h.writeError(c, opErr)
	}

	return c.JSON(http.StatusCreated, result)
}

func (h *Handler) handleGetARPTestResult(c *echo.Context, path string) error {
	policyARN, workflowID, testCaseID := extractARPBuildWorkflowTestCaseIDs(path)

	result, err := h.Backend.GetAutomatedReasoningPolicyTestResult(policyARN, workflowID, testCaseID)
	if err != nil {
		return h.writeError(c, err)
	}

	return c.JSON(http.StatusOK, result)
}

func (h *Handler) handleListARPTestResults(c *echo.Context, path string) error {
	policyARN, workflowID := extractARPWorkflowIDs(path)

	results, err := h.Backend.ListAutomatedReasoningPolicyTestResults(policyARN, workflowID)
	if err != nil {
		return h.writeError(c, err)
	}

	if results == nil {
		results = []map[string]any{}
	}

	return c.JSON(http.StatusOK, map[string]any{"testResults": results})
}

func (h *Handler) handleExportARPVersion(c *echo.Context, path string) error {
	arnParam := extractARPExportPolicyArn(path)

	result, err := h.Backend.ExportAutomatedReasoningPolicyVersion(arnParam)
	if err != nil {
		return h.writeError(c, err)
	}

	return c.JSON(http.StatusOK, result)
}

func (h *Handler) handleGetARPAnnotations(c *echo.Context, path string) error {
	policyARN, workflowID := extractARPWorkflowIDs(path)

	result, err := h.Backend.GetAutomatedReasoningPolicyAnnotations(policyARN, workflowID)
	if err != nil {
		return h.writeError(c, err)
	}

	return c.JSON(http.StatusOK, result)
}

type updateARPAnnotationsInput struct {
	LastUpdatedAnnotationSetHash string `json:"lastUpdatedAnnotationSetHash"`
	Annotations                  []any  `json:"annotations"`
}

// handleUpdateARPAnnotations updates annotations for a build workflow. Both
// annotations and lastUpdatedAnnotationSetHash are required
// (bedrock@v1.66.4 api_op_UpdateAutomatedReasoningPolicyAnnotations.go:37-56)
// and the real response carries annotationSetHash/buildWorkflowId/policyArn/
// updatedAt, all required too -- previously this returned an empty 200 body,
// dropping both the request and the response.
func (h *Handler) handleUpdateARPAnnotations(c *echo.Context, path string, body []byte) error {
	policyARN, workflowID := extractARPWorkflowIDs(path)

	in, err := parseBody[updateARPAnnotationsInput](body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse("ValidationException", "invalid request body"))
	}

	if in.Annotations == nil {
		return c.JSON(http.StatusBadRequest, errorResponse("ValidationException", "annotations is required"))
	}

	if in.LastUpdatedAnnotationSetHash == "" {
		return c.JSON(
			http.StatusBadRequest,
			errorResponse("ValidationException", "lastUpdatedAnnotationSetHash is required"),
		)
	}

	result, opErr := h.Backend.UpdateAutomatedReasoningPolicyAnnotations(
		policyARN, workflowID, in.Annotations, in.LastUpdatedAnnotationSetHash,
	)
	if opErr != nil {
		return h.writeError(c, opErr)
	}

	return c.JSON(http.StatusOK, result)
}

func (h *Handler) handleGetARPNextScenario(c *echo.Context, path string) error {
	policyARN, workflowID := extractARPWorkflowIDs(path)

	result, err := h.Backend.GetAutomatedReasoningPolicyNextScenario(policyARN, workflowID)
	if err != nil {
		return h.writeError(c, err)
	}

	return c.JSON(http.StatusOK, result)
}
