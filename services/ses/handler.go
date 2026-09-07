package ses

import (
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/pkgs/worker"
)

const (
	sesVersion    = "2010-12-01"
	sesXMLNS      = "http://ses.amazonaws.com/doc/2010-12-01/"
	unknownAction = "Unknown"
)

const (
	// boolTrue / boolFalse are the string literals used in AWS form-encoded params.
	boolTrue  = "true"
	boolFalse = "false"
)

// Handler is the Echo HTTP handler for SES operations.
type Handler struct {
	Backend    StorageBackend
	janitor    *Janitor
	janitorRun worker.SingleRun
}

// NewHandler creates a new SES handler with the given backend and logger.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{Backend: backend}
}

// WithJanitor attaches a background janitor to the handler.
// The janitor periodically evicts emails older than the backend TTL.
// interval=0 uses the default interval.
// The optional taskTimeout bounds each sweep; 0 means no per-task timeout.
func (h *Handler) WithJanitor(interval time.Duration, taskTimeout ...time.Duration) *Handler {
	ib, ok := h.Backend.(*InMemoryBackend)
	if !ok {
		return h
	}

	j := NewJanitor(ib, interval)
	if len(taskTimeout) > 0 {
		j.TaskTimeout = taskTimeout[0]
	}

	h.janitor = j

	return h
}

// StartWorker starts the background janitor if configured.
func (h *Handler) StartWorker(ctx context.Context) error {
	if h.janitor == nil {
		return nil
	}

	h.janitorRun.Start(ctx, h.janitor)

	return nil
}

// Shutdown stops the janitor worker and waits for it to exit.
func (h *Handler) Shutdown(ctx context.Context) {
	h.janitorRun.Stop(ctx)
}

// Reset clears all in-memory state. Used by the POST /_gopherstack/reset endpoint.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// Name returns the service name.
func (h *Handler) Name() string {
	return "SES"
}

var (
	_ service.BackgroundWorker = (*Handler)(nil)
	_ service.Shutdowner       = (*Handler)(nil)
)

// GetSupportedOperations returns the list of supported SES operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"CloneReceiptRuleSet",
		"CreateConfigurationSet",
		"CreateConfigurationSetEventDestination",
		"CreateConfigurationSetTrackingOptions",
		"CreateCustomVerificationEmailTemplate",
		"CreateReceiptFilter",
		"CreateReceiptRule",
		"CreateReceiptRuleSet",
		"CreateTemplate",
		"DeleteConfigurationSet",
		"DeleteConfigurationSetEventDestination",
		"DeleteConfigurationSetTrackingOptions",
		"DeleteCustomVerificationEmailTemplate",
		"DeleteIdentity",
		"DeleteIdentityPolicy",
		"DeleteReceiptFilter",
		"DeleteReceiptRule",
		"DeleteReceiptRuleSet",
		"DeleteTemplate",
		"DeleteVerifiedEmailAddress",
		"DescribeActiveReceiptRuleSet",
		"DescribeConfigurationSet",
		"DescribeReceiptRule",
		"DescribeReceiptRuleSet",
		"GetAccountSendingEnabled",
		"GetCustomVerificationEmailTemplate",
		"GetIdentityDkimAttributes",
		"GetIdentityMailFromDomainAttributes",
		"GetIdentityNotificationAttributes",
		"GetIdentityPolicies",
		"GetIdentityVerificationAttributes",
		"GetSendQuota",
		"GetSendStatistics",
		"GetTemplate",
		"ListConfigurationSets",
		"ListCustomVerificationEmailTemplates",
		"ListIdentities",
		"ListIdentityPolicies",
		"ListReceiptFilters",
		"ListReceiptRuleSets",
		"ListTemplates",
		"ListVerifiedEmailAddresses",
		"PutConfigurationSetDeliveryOptions",
		"PutIdentityPolicy",
		"ReorderReceiptRuleSet",
		"SendBounce",
		"SendBulkTemplatedEmail",
		"SendCustomVerificationEmail",
		"SendEmail",
		"SendRawEmail",
		"SendTemplatedEmail",
		"SetActiveReceiptRuleSet",
		"SetIdentityDkimEnabled",
		"SetIdentityFeedbackForwardingEnabled",
		"SetIdentityHeadersInNotificationsEnabled",
		"SetIdentityMailFromDomain",
		"SetIdentityNotificationTopic",
		"SetReceiptRulePosition",
		"TestRenderTemplate",
		"UpdateAccountSendingEnabled",
		"UpdateConfigurationSetEventDestination",
		"UpdateConfigurationSetReputationMetricsEnabled",
		"UpdateConfigurationSetSendingEnabled",
		"UpdateConfigurationSetTrackingOptions",
		"UpdateCustomVerificationEmailTemplate",
		"UpdateReceiptRule",
		"UpdateTemplate",
		"VerifyDomainDkim",
		"VerifyDomainIdentity",
		"VerifyEmailAddress",
		"VerifyEmailIdentity",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "ses" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this SES instance handles.
func (h *Handler) ChaosRegions() []string { return []string{config.DefaultRegion} }

// RouteMatcher returns a function that matches SES requests.
// SES requests are form-encoded POSTs containing Version=2010-12-01 and an
// action from the SES supported operations list. We check both the version and
// action to avoid routing conflicts with Elastic Beanstalk, which also uses
// Version=2010-12-01 but with a disjoint set of action names.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		r := c.Request()
		if r.Method != http.MethodPost {
			return false
		}

		if strings.HasPrefix(r.URL.Path, "/dashboard/") {
			return false
		}

		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/x-www-form-urlencoded") {
			return false
		}

		body, err := httputils.ReadBody(r)
		if err != nil {
			// Body unreadable (e.g. oversized): fall back to the User-Agent
			// marker every aws-sdk-go-v2 ses client sets (api_client.go's
			// AddSDKAgentKeyValue -- "api/ses"). That still identifies this
			// as ours, so claim it and let Handler() produce the typed
			// error instead of masking the read failure as a 404.
			return service.MatchesUserAgentMarker(r.Header, "api/ses")
		}

		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return false
		}

		return vals.Get("Version") == sesVersion && slices.Contains(h.GetSupportedOperations(), vals.Get("Action"))
	}
}

// MatchPriority returns the routing priority for the SES handler.
func (h *Handler) MatchPriority() int {
	return service.PriorityFormStandard
}

// ExtractOperation extracts the SES action from the request body.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return unknownAction
	}

	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return unknownAction
	}

	action := vals.Get("Action")
	if action == "" {
		return unknownAction
	}

	return action
}

// ExtractResource returns the source email address or identity from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return ""
	}

	for _, key := range []string{"Source", "EmailAddress", "Identity", "RuleSetName", "FilterName"} {
		if v := vals.Get(key); v != "" {
			return v
		}
	}

	return ""
}

// Handler returns the Echo handler function for SES requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		reqID := newRequestID()

		r := c.Request()
		body, err := httputils.ReadBody(r)
		if err != nil {
			log.ErrorContext(ctx, "failed to read SES request body", "error", err)

			return h.writeError(
				c,
				reqID,
				http.StatusInternalServerError,
				"InternalFailure",
				"failed to read request body",
			)
		}

		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return h.writeError(
				c,
				reqID,
				http.StatusInternalServerError,
				"InternalFailure",
				"failed to parse request body",
			)
		}

		action := vals.Get("Action")
		if action == "" {
			return h.writeError(c, reqID, http.StatusBadRequest, "MissingAction", "missing Action parameter")
		}

		log.DebugContext(ctx, "SES request", "action", action)

		resp, opErr := h.dispatch(vals, reqID, action)

		switch {
		case errors.Is(opErr, errUnknownSESAction):
			return h.writeError(c, reqID, http.StatusBadRequest, "InvalidAction",
				action+" is not a valid SES action")
		case opErr != nil:
			return h.handleOpError(c, reqID, action, opErr)
		}

		xmlBytes, marshalErr := marshalXML(resp)
		if marshalErr != nil {
			log.ErrorContext(ctx, "failed to marshal SES response", "action", action, "error", marshalErr)

			return h.writeError(c, reqID, http.StatusInternalServerError, "InternalFailure", "internal server error")
		}

		return c.Blob(http.StatusOK, "text/xml", xmlBytes)
	}
}

// errUnknownSESAction is returned by dispatch when the action is not recognised.
var errUnknownSESAction = errors.New("unknown SES action")

// dispatch routes a parsed SES action to the appropriate handler.
func (h *Handler) dispatch(vals url.Values, reqID, action string) (any, error) {
	switch action {
	case "VerifyEmailIdentity":
		return h.handleVerifyEmailIdentity(vals, reqID)
	case "DeleteIdentity":
		return h.handleDeleteIdentity(vals, reqID), nil
	case "ListIdentities":
		return h.handleListIdentities(vals, reqID)
	case "GetIdentityVerificationAttributes":
		return h.handleGetIdentityVerificationAttributes(vals, reqID), nil
	case "GetAccountSendingEnabled":
		return h.handleGetAccountSendingEnabled(reqID), nil
	case "SendEmail":
		return h.handleSendEmail(vals, reqID)
	case "SendRawEmail":
		return h.handleSendRawEmail(vals, reqID)
	case "SendTemplatedEmail":
		return h.handleSendTemplatedEmail(vals, reqID)
	default:
		return h.dispatchExtended(vals, reqID, action)
	}
}

// dispatchExtended handles the template/config-set/stats/receipt operations.
func (h *Handler) dispatchExtended(vals url.Values, reqID, action string) (any, error) {
	switch action {
	case "CreateTemplate":
		return h.handleCreateTemplate(vals, reqID)
	case "UpdateTemplate":
		return h.handleUpdateTemplate(vals, reqID)
	case "GetTemplate":
		return h.handleGetTemplate(vals, reqID)
	case "ListTemplates":
		return h.handleListTemplates(vals, reqID), nil
	case "DeleteTemplate":
		return h.handleDeleteTemplate(vals, reqID), nil
	case "CreateConfigurationSet":
		return h.handleCreateConfigurationSet(vals, reqID)
	case "DeleteConfigurationSet":
		return h.handleDeleteConfigurationSet(vals, reqID)
	case "ListConfigurationSets":
		return h.handleListConfigurationSets(vals, reqID), nil
	case "GetSendQuota":
		return h.handleGetSendQuota(reqID), nil
	case "GetSendStatistics":
		return h.handleGetSendStatistics(reqID), nil
	default:
		return h.dispatchNewOps(vals, reqID, action)
	}
}

// dispatchNewOps handles receipt rule sets, filters, event destinations, tracking options,
// and custom verification email template operations.
func (h *Handler) dispatchNewOps(vals url.Values, reqID, action string) (any, error) {
	switch action {
	case "CreateReceiptRuleSet":
		return h.handleCreateReceiptRuleSet(vals, reqID)
	case "CloneReceiptRuleSet":
		return h.handleCloneReceiptRuleSet(vals, reqID)
	case "CreateReceiptRule":
		return h.handleCreateReceiptRule(vals, reqID)
	case "CreateReceiptFilter":
		return h.handleCreateReceiptFilter(vals, reqID)
	case "CreateConfigurationSetEventDestination":
		return h.handleCreateConfigurationSetEventDestination(vals, reqID)
	case "DeleteConfigurationSetEventDestination":
		return h.handleDeleteConfigurationSetEventDestination(vals, reqID)
	case "CreateConfigurationSetTrackingOptions":
		return h.handleCreateConfigurationSetTrackingOptions(vals, reqID)
	case "DeleteConfigurationSetTrackingOptions":
		return h.handleDeleteConfigurationSetTrackingOptions(vals, reqID)
	case "CreateCustomVerificationEmailTemplate":
		return h.handleCreateCustomVerificationEmailTemplate(vals, reqID)
	case "DeleteCustomVerificationEmailTemplate":
		return h.handleDeleteCustomVerificationEmailTemplate(vals, reqID)
	default:
		return h.dispatchRefinedOps(vals, reqID, action)
	}
}

// dispatchRefinedOps handles the newer receipt filter/rule set query and active rule set operations.
func (h *Handler) dispatchRefinedOps(vals url.Values, reqID, action string) (any, error) {
	switch action {
	case "ListReceiptFilters":
		return h.handleListReceiptFilters(reqID), nil
	case "ListReceiptRuleSets":
		return h.handleListReceiptRuleSets(vals, reqID), nil
	case "DeleteReceiptFilter":
		return h.handleDeleteReceiptFilter(vals, reqID)
	case "DeleteReceiptRule":
		return h.handleDeleteReceiptRule(vals, reqID)
	case "DeleteReceiptRuleSet":
		return h.handleDeleteReceiptRuleSet(vals, reqID)
	case "GetCustomVerificationEmailTemplate":
		return h.handleGetCustomVerificationEmailTemplate(vals, reqID)
	case "ListCustomVerificationEmailTemplates":
		return h.handleListCustomVerificationEmailTemplates(vals, reqID), nil
	case "DescribeReceiptRuleSet":
		return h.handleDescribeReceiptRuleSet(vals, reqID)
	case "SetActiveReceiptRuleSet":
		return h.handleSetActiveReceiptRuleSet(vals, reqID)
	case "DescribeActiveReceiptRuleSet":
		return h.handleDescribeActiveReceiptRuleSet(reqID)
	default:
		return h.dispatchMissingOps(vals, reqID, action)
	}
}

// dispatchMissingOps handles the previously missing SES operations.
// It delegates to three sub-dispatchers by domain to keep cyclomatic complexity low.
func (h *Handler) dispatchMissingOps(vals url.Values, reqID, action string) (any, error) {
	res, err := h.dispatchIdentityOps(vals, reqID, action)
	if !errIsUnknown(err) {
		return res, err
	}

	res, err = h.dispatchSendMissingOps(vals, reqID, action)
	if !errIsUnknown(err) {
		return res, err
	}

	return h.dispatchConfigReceiptOps(vals, reqID, action)
}

// errIsUnknown reports whether err is errUnknownSESAction.
func errIsUnknown(err error) bool {
	return errors.Is(err, errUnknownSESAction)
}

// dispatchIdentityOps handles identity policy, attribute, domain verification and
// legacy email address operations.
// dispatchIdentityOps handles identity policy and attribute operations.
func (h *Handler) dispatchIdentityOps(vals url.Values, reqID, action string) (any, error) {
	switch action {
	case "PutIdentityPolicy":
		return h.handlePutIdentityPolicy(vals, reqID)

	case "DeleteIdentityPolicy":
		return h.handleDeleteIdentityPolicy(vals, reqID)

	case "GetIdentityPolicies":
		return h.handleGetIdentityPolicies(vals, reqID)

	case "ListIdentityPolicies":
		return h.handleListIdentityPolicies(vals, reqID)

	case "GetIdentityDkimAttributes":
		return h.handleGetIdentityDkimAttributes(vals, reqID), nil

	case "GetIdentityMailFromDomainAttributes":
		return h.handleGetIdentityMailFromDomainAttributes(vals, reqID), nil

	case "GetIdentityNotificationAttributes":
		return h.handleGetIdentityNotificationAttributes(vals, reqID), nil

	case "SetIdentityDkimEnabled":
		return h.handleSetIdentityDkimEnabled(vals, reqID)

	case "SetIdentityFeedbackForwardingEnabled":
		return h.handleSetIdentityFeedbackForwardingEnabled(vals, reqID)

	default:
		return h.dispatchIdentitySetVerifyOps(vals, reqID, action)
	}
}

// dispatchIdentitySetVerifyOps handles the identity set/notification, domain verification and
// legacy email address operations.
func (h *Handler) dispatchIdentitySetVerifyOps(vals url.Values, reqID, action string) (any, error) {
	switch action {
	case "SetIdentityHeadersInNotificationsEnabled":
		return h.handleSetIdentityHeadersInNotificationsEnabled(vals, reqID)

	case "SetIdentityMailFromDomain":
		return h.handleSetIdentityMailFromDomain(vals, reqID)

	case "SetIdentityNotificationTopic":
		return h.handleSetIdentityNotificationTopic(vals, reqID)

	case "VerifyDomainIdentity":
		return h.handleVerifyDomainIdentity(vals, reqID)

	case "VerifyDomainDkim":
		return h.handleVerifyDomainDkim(vals, reqID)

	case "VerifyEmailAddress":
		return h.handleVerifyEmailAddress(vals, reqID)

	case "DeleteVerifiedEmailAddress":
		return h.handleDeleteVerifiedEmailAddress(vals, reqID), nil

	case "ListVerifiedEmailAddresses":
		return h.handleListVerifiedEmailAddresses(reqID), nil

	case "UpdateAccountSendingEnabled":
		return h.handleUpdateAccountSendingEnabled(vals, reqID), nil

	default:
		return nil, errUnknownSESAction
	}
}

// dispatchSendMissingOps handles the new send/render/custom-verif-template operations.
func (h *Handler) dispatchSendMissingOps(vals url.Values, reqID, action string) (any, error) {
	switch action {
	case "SendBounce":
		return h.handleSendBounce(vals, reqID)

	case "SendBulkTemplatedEmail":
		return h.handleSendBulkTemplatedEmail(vals, reqID)

	case "SendCustomVerificationEmail":
		return h.handleSendCustomVerificationEmail(vals, reqID)

	case "TestRenderTemplate":
		return h.handleTestRenderTemplate(vals, reqID)

	case "UpdateCustomVerificationEmailTemplate":
		return h.handleUpdateCustomVerificationEmailTemplate(vals, reqID)

	default:
		return nil, errUnknownSESAction
	}
}

// dispatchConfigReceiptOps handles the receipt rule and configuration set update operations.
func (h *Handler) dispatchConfigReceiptOps(vals url.Values, reqID, action string) (any, error) {
	switch action {
	case "DescribeReceiptRule":
		return h.handleDescribeReceiptRule(vals, reqID)

	case "UpdateReceiptRule":
		return h.handleUpdateReceiptRule(vals, reqID)

	case "ReorderReceiptRuleSet":
		return h.handleReorderReceiptRuleSet(vals, reqID)

	case "SetReceiptRulePosition":
		return h.handleSetReceiptRulePosition(vals, reqID)

	case "DescribeConfigurationSet":
		return h.handleDescribeConfigurationSet(vals, reqID)

	case "PutConfigurationSetDeliveryOptions":
		return h.handlePutConfigurationSetDeliveryOptions(vals, reqID)

	case "UpdateConfigurationSetEventDestination":
		return h.handleUpdateConfigurationSetEventDestination(vals, reqID)

	case "UpdateConfigurationSetReputationMetricsEnabled":
		return h.handleUpdateConfigurationSetReputationMetricsEnabled(vals, reqID)

	case "UpdateConfigurationSetSendingEnabled":
		return h.handleUpdateConfigurationSetSendingEnabled(vals, reqID)

	case "UpdateConfigurationSetTrackingOptions":
		return h.handleUpdateConfigurationSetTrackingOptions(vals, reqID)

	default:
		return nil, errUnknownSESAction
	}
}

const errCodeAlreadyExists = "AlreadyExists"

// sesErrorCode maps an operation error to the SES XML error code and HTTP status.
// Returns empty string if the error is unrecognised (caller should use InternalFailure).
func sesErrorCode(opErr error) (string, int) {
	status := http.StatusBadRequest

	switch {
	case errors.Is(opErr, ErrInvalidParameter):
		return "InvalidParameterValue", status
	case errors.Is(opErr, ErrInvalidPolicy):
		return "InvalidPolicy", status
	case errors.Is(opErr, ErrMessageRejected):
		return "MessageRejected", status
	case errors.Is(opErr, ErrTemplateNotFound):
		return "TemplateDoesNotExist", status
	case errors.Is(opErr, ErrTemplateExists):
		return errCodeAlreadyExists, status
	case errors.Is(opErr, ErrConfigSetNotFound):
		return "ConfigurationSetDoesNotExist", status
	case errors.Is(opErr, ErrConfigSetExists):
		return "ConfigurationSetAlreadyExists", status
	case errors.Is(opErr, ErrAccountSendingPaused):
		return "AccountSendingPausedException", status
	case errors.Is(opErr, ErrValidation):
		return "ValidationError", status
	}

	return sesNewOpsErrorCode(opErr, status)
}

// sesNewOpsErrorCode maps errors introduced by the new operations (receipt rules,
// filters, event destinations, tracking options, custom verification templates).
func sesNewOpsErrorCode(opErr error, status int) (string, int) {
	switch {
	case errors.Is(opErr, ErrReceiptRuleSetNotFound):
		return "RuleSetDoesNotExist", status
	case errors.Is(opErr, ErrReceiptRuleSetExists):
		return errCodeAlreadyExists, status
	case errors.Is(opErr, ErrReceiptRuleSetActive):
		return "CannotDelete", status
	case errors.Is(opErr, ErrReceiptRuleNotFound):
		return "RuleDoesNotExist", status
	case errors.Is(opErr, ErrReceiptRuleExists):
		return errCodeAlreadyExists, status
	case errors.Is(opErr, ErrReceiptFilterExists):
		return errCodeAlreadyExists, status
	case errors.Is(opErr, ErrEventDestinationNotFound):
		return "EventDestinationDoesNotExist", status
	case errors.Is(opErr, ErrEventDestinationExists):
		return "EventDestinationAlreadyExists", status
	case errors.Is(opErr, ErrTrackingOptionsNotFound):
		return "TrackingOptionsDoesNotExistException", status
	case errors.Is(opErr, ErrTrackingOptionsExists):
		return "TrackingOptionsAlreadyExistsException", status
	case errors.Is(opErr, ErrCustomVerifTemplateNotFound):
		return "CustomVerificationEmailTemplateDoesNotExist", status
	case errors.Is(opErr, ErrCustomVerifTemplateExists):
		return "CustomVerificationEmailTemplateAlreadyExists", status
	default:
		return "", http.StatusInternalServerError
	}
}

func (h *Handler) handleOpError(c *echo.Context, reqID, action string, opErr error) error {
	code, statusCode := sesErrorCode(opErr)
	if code == "" {
		code = "InternalFailure"
		logger.Load(c.Request().Context()).Error("SES internal error", "error", opErr, "action", action)
	}

	return h.writeError(c, reqID, statusCode, code, opErr.Error())
}

func (h *Handler) writeError(c *echo.Context, reqID string, statusCode int, code, message string) error {
	errResp := &sesErrorResponse{
		Xmlns:     sesXMLNS,
		Error:     sesError{Code: code, Message: message, Type: "Sender"},
		RequestID: reqID,
	}

	xmlBytes, err := marshalXML(errResp)
	if err != nil {
		return c.String(http.StatusInternalServerError, "internal server error")
	}

	return c.Blob(statusCode, "text/xml", xmlBytes)
}

// marshalXML encodes the payload with the XML declaration header.
func marshalXML(v any) ([]byte, error) {
	raw, err := xml.Marshal(v)
	if err != nil {
		return nil, err
	}

	return append([]byte(xml.Header), raw...), nil
}

// newRequestID generates a unique request ID for SES responses.
func newRequestID() string {
	return "gopherstack-" + uuid.New().String()
}

type sesError struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
	Type    string `xml:"Type"`
}

type sesErrorResponse struct {
	XMLName   xml.Name `xml:"ErrorResponse"`
	Xmlns     string   `xml:"xmlns,attr"`
	Error     sesError `xml:"Error"`
	RequestID string   `xml:"RequestId"`
}

type xmlMember struct {
	Value string `xml:",chardata"`
}

type xmlMemberList struct {
	Members []xmlMember `xml:"member"`
}

// parseSESMemberList parses form values like "Prefix.member.1", "Prefix.member.2".
func parseSESMemberList(vals url.Values, prefix string) []string {
	var result []string
	base := prefix + ".member."

	for i := 1; ; i++ {
		v := vals.Get(base + strconv.Itoa(i))
		if v == "" {
			return result
		}

		result = append(result, v)
	}
}

// parseSESTags parses Tags.member.N.{Name,Value} form values into a []Tag slice.
func parseSESTags(vals url.Values, prefix string) []Tag {
	var tags []Tag
	base := prefix + ".member."

	for i := 1; ; i++ {
		idx := base + strconv.Itoa(i)
		name := vals.Get(idx + ".Name")
		value := vals.Get(idx + ".Value")

		if name == "" && value == "" {
			break
		}

		tags = append(tags, Tag{Name: name, Value: value})
	}

	return tags
}

// emptyResult carries the dynamic "<Action>Result" wrapper element name for
// void-result operations whose SES wire format wraps an (empty) Result element.
// The XMLName field's tag is intentionally blank so xml.Marshal uses the
// runtime value set by the caller rather than a fixed literal name.
type emptyResult struct {
	XMLName xml.Name
}

// emptyResponse is a generic empty-result XML envelope used by no-op operations.
// Result is nil for actions whose SES output shape has zero members, so the
// real wire format omits the Result element entirely (e.g. VerifyEmailAddress).
// Otherwise it carries the per-op "<Action>Result" element name: real AWS SDK
// clients call decoder.GetElement("<Action>Result") before parsing the body, so
// a missing or misnamed wrapper causes a client-side DeserializationError even
// though the emulator's backend state mutation succeeded.
type emptyResponse struct {
	XMLName   xml.Name
	Xmlns     string       `xml:"xmlns,attr"`
	Result    *emptyResult `xml:",omitempty"`
	RequestID string       `xml:"ResponseMetadata>RequestId"`
}

// newEmptyResponseWithResult builds an emptyResponse for an action whose real
// SES output shape wraps an (empty) "<action>Result" element.
func newEmptyResponseWithResult(action, reqID string) *emptyResponse {
	return &emptyResponse{
		XMLName:   xml.Name{Local: action + "Response"},
		Xmlns:     sesXMLNS,
		Result:    &emptyResult{XMLName: xml.Name{Local: action + "Result"}},
		RequestID: reqID,
	}
}

// newEmptyResponse builds an emptyResponse for an action whose real SES output
// shape has zero members, so the wire format has no Result element at all.
func newEmptyResponse(action, reqID string) *emptyResponse {
	return &emptyResponse{
		XMLName:   xml.Name{Local: action + "Response"},
		Xmlns:     sesXMLNS,
		RequestID: reqID,
	}
}
