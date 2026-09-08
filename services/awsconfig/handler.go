package awsconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const awsConfigTargetPrefix = "StarlingDoveService."

var errUnknownAction = errors.New("unknown action")

type configurationRecorderNameInput struct {
	ConfigurationRecorderName string `json:"ConfigurationRecorderName"`
}

// emptyInput/emptyOutput are shared request/response placeholders for
// operations that carry no meaningful body, used across every op-family file.
type (
	emptyInput  struct{}
	emptyOutput struct{}
)

// Handler is the Echo HTTP handler for AWS Config operations.
type Handler struct {
	Backend       *InMemoryBackend
	dispatchTable map[string]service.JSONOpFunc
}

// NewHandler creates a new AWS Config handler.
func NewHandler(backend *InMemoryBackend) *Handler {
	h := &Handler{Backend: backend}
	h.dispatchTable = h.buildDispatchTable()

	return h
}

// Name returns the service name.
func (h *Handler) Name() string { return "AWSConfig" }

// GetSupportedOperations returns the list of supported AWS Config operations.
func (h *Handler) GetSupportedOperations() []string {
	families := [][]string{
		configurationRecorderSupportedOps(),
		deliveryChannelSupportedOps(),
		configRuleSupportedOps(),
		aggregatorSupportedOps(),
		conformancePackSupportedOps(),
		organizationSupportedOps(),
		retentionSupportedOps(),
		remediationSupportedOps(),
		storedQuerySupportedOps(),
		tagSupportedOps(),
		resourceSupportedOps(),
		connectorSupportedOps(),
	}

	total := 0
	for _, f := range families {
		total += len(f)
	}

	ops := make([]string, 0, total)
	for _, f := range families {
		ops = append(ops, f...)
	}

	return ops
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "config" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this AWS Config instance handles.
func (h *Handler) ChaosRegions() []string { return []string{config.DefaultRegion} }

// RouteMatcher returns a function that matches AWS Config requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), awsConfigTargetPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// ExtractOperation extracts the AWS Config action from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	action := strings.TrimPrefix(target, awsConfigTargetPrefix)
	if action == "" || action == target {
		return "Unknown"
	}

	return action
}

// extractResourceFieldByOp maps an operation to the single top-level JSON
// field ExtractResource should read as its resource identifier, for every
// operation whose resource is a plain extractNamedField(body, key) lookup.
// Operations needing a more specialized extractor (the recorder/delivery-
// channel name variants) are handled directly in ExtractResource's switch
// instead, since they don't fit this "one op -> one field" shape. Data-driven
// instead of more switch cases so adding a new simple-field op never grows
// ExtractResource's cyclomatic complexity.
//
// resourceFieldConfigRuleName/resourceFieldArn are field names repeated
// across multiple extractResourceFieldByOp entries, factored out so goconst
// doesn't flag the map literal's repeated string values.
const (
	resourceFieldConfigRuleName = "ConfigRuleName"
	resourceFieldArn            = "Arn"
)

//nolint:gochecknoglobals // lookup table, analogous to errorWireMappings above
var extractResourceFieldByOp = map[string]string{
	opDeleteConfigRule:                                resourceFieldConfigRuleName,
	opDescribeConfigRules:                             resourceFieldConfigRuleName,
	opGetComplianceDetailsByConfigRule:                resourceFieldConfigRuleName,
	opDeleteEvaluationResults:                         resourceFieldConfigRuleName,
	opDeleteConfigurationAggregator:                   "ConfigurationAggregatorName",
	opBatchGetAggregateResourceConfig:                 "ConfigurationAggregatorName",
	opDeleteConformancePack:                           "ConformancePackName",
	opDeleteOrganizationConfigRule:                    "OrganizationConfigRuleName",
	opDeleteOrganizationConformancePack:               "OrganizationConformancePackName",
	opAssociateResourceTypes:                          "ConfigurationRecorderArn",
	opGetConnector:                                    resourceFieldArn,
	opDeleteConnector:                                 resourceFieldArn,
	opPutThirdPartyServiceLinkedConfigurationRecorder: "ServicePrincipal",
}

// ExtractResource extracts a resource identifier from the request body based on the operation.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	op := h.ExtractOperation(c)

	switch op {
	case opPutConfigurationRecorder:
		return extractConfigRecorderName(body)
	case opStartConfigurationRecorder, opStopConfigurationRecorder, opDeleteConfigurationRecorder:
		return extractTopLevelRecorderName(body)
	case opDescribeConfigurationRecorders, opDescribeConfigurationRecorderStatus:
		return extractFirstRecorderName(body)
	case opPutDeliveryChannel:
		return extractDeliveryChannelName(body)
	case opDescribeDeliveryChannels, opDeleteDeliveryChannel:
		return extractFirstDeliveryChannelName(body)
	}

	if field, ok := extractResourceFieldByOp[op]; ok {
		return extractNamedField(body, field)
	}

	return extractTopLevelRecorderName(body)
}

// configNameBody is a JSON wrapper carrying a single "name" field, used for both
// configuration recorder and delivery channel name extraction.
type configNameBody struct {
	Name string `json:"name"`
}

// extractNamedField extracts a top-level string field by key from a JSON body.
func extractNamedField(body []byte, key string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}

	raw, ok := m[key]
	if !ok {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}

	return s
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return service.HandleTarget(
			c, logger.Load(c.Request().Context()),
			"AWSConfig", "application/x-amz-json-1.1",
			h.GetSupportedOperations(),
			h.dispatch,
			h.handleError,
		)
	}
}

// buildDispatchTable assembles the full operation dispatch table from every
// op-family's own dispatch builder.
func (h *Handler) buildDispatchTable() map[string]service.JSONOpFunc {
	builders := []func() map[string]service.JSONOpFunc{
		h.buildConfigurationRecorderDispatch,
		h.buildDeliveryChannelDispatch,
		h.buildConfigRuleDispatch,
		h.buildAggregatorDispatch,
		h.buildConformancePackDispatch,
		h.buildOrganizationDispatch,
		h.buildRetentionDispatch,
		h.buildRemediationDispatch,
		h.buildStoredQueryDispatch,
		h.buildTagDispatch,
		h.buildResourceDispatch,
		h.buildConnectorDispatch,
	}

	table := make(map[string]service.JSONOpFunc)

	for _, build := range builders {
		maps.Copy(table, build())
	}

	return table
}

func (h *Handler) dispatch(ctx context.Context, action string, body []byte) ([]byte, error) {
	fn, ok := h.dispatchTable[action]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUnknownAction, action)
	}

	result, err := fn(ctx, body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

// marshalError serialises a typed AWS error response into bytes.
// Marshaling a struct with only string fields cannot fail; error is intentionally ignored.
func marshalError(errType, message string) []byte {
	payload, _ := json.Marshal(service.JSONErrorResponse{Type: errType, Message: message})

	return payload
}

// errorWireMapping associates a sentinel error with the AWS wire error type
// and HTTP status real AWS Config uses for it.
type errorWireMapping struct {
	sentinel   error
	wireType   string
	httpStatus int
}

// errorWireMappings is the ordered sentinel -> (wireType, httpStatus) table
// handleError walks. Data-driven instead of a long switch so adding a new
// sentinel/wire-type pair never grows handleError's cyclomatic complexity.
//
//nolint:gochecknoglobals // lookup table, analogous to errCodeLookup-style lookup tables elsewhere
var errorWireMappings = []errorWireMapping{
	{ErrNotFound, "NoSuchConfigurationRecorderException", http.StatusNotFound},
	{ErrNoSuchDeliveryChannel, "NoSuchDeliveryChannelException", http.StatusNotFound},
	{ErrNoSuchConfigRule, "NoSuchConfigRuleException", http.StatusNotFound},
	{ErrNoSuchAggregator, "NoSuchConfigurationAggregatorException", http.StatusNotFound},
	{ErrNoSuchConformancePack, "NoSuchConformancePackException", http.StatusNotFound},
	{ErrNoSuchOrganizationConfigRule, "NoSuchOrganizationConfigRuleException", http.StatusNotFound},
	{ErrNoSuchOrganizationConformancePack, "NoSuchOrganizationConformancePackException", http.StatusNotFound},
	{ErrResourceNotFound, "ResourceNotFoundException", http.StatusBadRequest},
	{ErrAlreadyExists, "MaxNumberOfConfigurationRecordersExceededException", http.StatusConflict},
	{ErrNoDeliveryChannel, "NoAvailableDeliveryChannelException", http.StatusBadRequest},
	{ErrNoSuchConfigRuleInConformancePack, "NoSuchConfigRuleInConformancePackException", http.StatusNotFound},
	{ErrNoSuchRemediationConfiguration, "NoSuchRemediationConfigurationException", http.StatusNotFound},
	{ErrNoAvailableConfigurationRecorder, "NoAvailableConfigurationRecorderException", http.StatusBadRequest},
	{ErrNoRunningConfigurationRecorder, "NoRunningConfigurationRecorderException", http.StatusBadRequest},
	{ErrInvalidConfigurationRecorderName, "InvalidConfigurationRecorderNameException", http.StatusBadRequest},
	{ErrInvalidRole, "InvalidRoleException", http.StatusBadRequest},
	{ErrInvalidDeliveryChannelName, "InvalidDeliveryChannelNameException", http.StatusBadRequest},
	{ErrConflict, "ConflictException", http.StatusBadRequest},
	{ErrValidation, "ValidationException", http.StatusBadRequest},
	{ErrInvalidParameterValue, "InvalidParameterValueException", http.StatusBadRequest},
	{ErrInvalidNextToken, "InvalidNextTokenException", http.StatusBadRequest},
	{ErrLastDeliveryChannelDeleteFailed, "LastDeliveryChannelDeleteFailedException", http.StatusBadRequest},
}

func (h *Handler) handleError(_ context.Context, c *echo.Context, _ string, err error) error {
	for _, m := range errorWireMappings {
		if errors.Is(err, m.sentinel) {
			return c.JSONBlob(m.httpStatus, marshalError(m.wireType, err.Error()))
		}
	}

	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError

	// configservice@v1.68.4 has no single code that fits every operation here:
	// ValidationException is modeled on only 38 of its ~102 operations, the rest
	// use InvalidParameterValueException or model no bad-request-class error at
	// all (e.g. DeleteConfigurationAggregator declares only
	// NoSuchConfigurationAggregatorException). Left untyped rather than guessing.
	if errors.Is(err, errUnknownAction) || errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": err.Error()})
	}

	// configservice@v1.68.4's types/errors.go declares no internal-server/
	// generic-server exception at all -- left untyped rather than inventing one.
	return c.JSON(http.StatusInternalServerError, map[string]string{"message": err.Error()})
}
