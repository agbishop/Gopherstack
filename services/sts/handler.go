package sts

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	contentTypeForm   = "application/x-www-form-urlencoded"
	stsVersion        = "Version=2011-06-15"
	unknownOperation  = "Unknown"
	invalidAction     = "InvalidAction"
	validationError   = "ValidationError"
	invalidParamValue = "InvalidParameterValue"
	kvPairLen         = 2
)

// Handler is the Echo HTTP handler for STS operations.
type Handler struct {
	Backend StorageBackend
	janitor *Janitor
}

// NewHandler creates a new STS handler with the given backend.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{
		Backend: backend,
	}
}

// WithJanitor attaches a background janitor to the handler.
// The janitor periodically evicts expired sessions. interval=0 uses the default.
// The optional taskTimeout bounds each sweep; 0 means no per-task timeout.
func (h *Handler) WithJanitor(interval time.Duration, taskTimeout ...time.Duration) *Handler {
	if memBackend, ok := h.Backend.(*InMemoryBackend); ok {
		j := NewJanitor(memBackend, interval)
		if len(taskTimeout) > 0 {
			j.TaskTimeout = taskTimeout[0]
		}

		h.janitor = j
	}

	return h
}

// StartWorker starts the background janitor if it is configured.
func (h *Handler) StartWorker(ctx context.Context) error {
	if h.janitor != nil {
		go h.janitor.Run(ctx)
	}

	return nil
}

// Name returns the service name.
func (h *Handler) Name() string {
	return "STS"
}

// GetSupportedOperations returns the list of supported STS operations.
// GetDelegatedAccessToken and GetWebIdentityToken are real actions present in
// aws-sdk-go-v2/service/sts (api_op_GetDelegatedAccessToken.go and
// api_op_GetWebIdentityToken.go); both are included here and are routed by
// dispatch.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"AssumeRole",
		"AssumeRoleWithSAML",
		"AssumeRoleWithWebIdentity",
		"AssumeRoot",
		"DecodeAuthorizationMessage",
		"GetAccessKeyInfo",
		"GetCallerIdentity",
		"GetDelegatedAccessToken",
		"GetFederationToken",
		"GetSessionToken",
		"GetWebIdentityToken",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "sts" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this STS instance handles.
func (h *Handler) ChaosRegions() []string { return []string{config.DefaultRegion} }

// RouteMatcher returns a matcher that identifies STS requests by Content-Type and Version.
// Dashboard paths are excluded so that browser form submissions (Playwright tests)
// are not intercepted by the STS handler.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path
		if strings.HasPrefix(path, "/dashboard/") || path == "/dashboard" {
			return false
		}

		ct := strings.ToLower(c.Request().Header.Get("Content-Type"))
		if !strings.Contains(ct, contentTypeForm) {
			return false
		}

		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			// Body unreadable (e.g. oversized): fall back to the User-Agent
			// marker every aws-sdk-go-v2 sts client sets (api_client.go's
			// AddSDKAgentKeyValue -- "api/sts"). That still identifies this
			// as ours, so claim it and let Handler() produce the typed
			// error instead of masking the read failure as a 404.
			return service.MatchesUserAgentMarker(c.Request().Header, "api/sts")
		}

		return strings.Contains(string(body), stsVersion)
	}
}

// MatchPriority returns the routing priority for the STS handler.
func (h *Handler) MatchPriority() int {
	return service.PriorityFormEncoded
}

// ExtractOperation reads the Action parameter from the request body.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return unknownOperation
	}

	// Parse as form values manually to avoid consuming the body permanently.
	values := parseFormValues(body)
	action := values["Action"]

	if action == "" {
		return unknownOperation
	}

	return action
}

// ExtractResource returns the RoleArn for AssumeRole calls, empty otherwise.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	values := parseFormValues(body)

	return values["RoleArn"]
}

// Handler returns the Echo handler function for STS operations.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		if c.Request().Method == http.MethodGet {
			return c.JSON(http.StatusOK, h.GetSupportedOperations())
		}

		if c.Request().Method != http.MethodPost {
			return h.handleError(ctx, c, fmt.Errorf("%w: method not allowed", ErrValidation))
		}

		response, err := h.dispatch(ctx, c.Request())
		if err != nil {
			return h.handleError(ctx, c, err)
		}

		log.DebugContext(ctx, "STS request completed")

		return writeXMLResponse(c, http.StatusOK, response)
	}
}

// dispatch parses the STS request and calls the appropriate backend method.
func (h *Handler) dispatch(ctx context.Context, r *http.Request) (any, error) {
	log := logger.Load(ctx)

	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("parse form: %w", err)
	}

	action := r.FormValue("Action")
	if action == "" {
		return nil, ErrMissingAction
	}

	log.DebugContext(ctx, "STS request", "action", action)

	switch action {
	case "AssumeRole":
		return h.dispatchAssumeRole(r)
	case "AssumeRoleWithSAML":
		return h.dispatchAssumeRoleWithSAML(r)
	case "AssumeRoleWithWebIdentity":
		return h.dispatchAssumeRoleWithWebIdentity(r)
	case "AssumeRoot":
		return h.dispatchAssumeRoot(r)
	case "GetCallerIdentity":
		return h.Backend.GetCallerIdentity(
			extractAccessKeyFromAuth(r),
			r.Header.Get("X-Amz-Security-Token"),
		)
	case "GetDelegatedAccessToken":
		return h.dispatchGetDelegatedAccessToken(r)
	case "GetFederationToken":
		return h.dispatchGetFederationToken(r)
	case "GetSessionToken":
		return h.dispatchGetSessionToken(r)
	case "GetWebIdentityToken":
		return h.dispatchGetWebIdentityToken(r)
	case "GetAccessKeyInfo":
		return h.dispatchGetAccessKeyInfo(r)
	case "DecodeAuthorizationMessage":
		return h.dispatchDecodeAuthorizationMessage(r)
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidAction, action)
	}
}

// mapErrorToCode maps a known STS error to its AWS error code and HTTP status.
// Returns ("", 0) when the error is not a known client error. Split into two
// helpers (mapValidationErrorToCode / mapNamedExceptionToCode) purely to keep
// cyclomatic complexity under the linter's threshold — the two groups have no
// other semantic relationship, so either helper being extended independently
// is fine.
func mapErrorToCode(reqErr error) (string, int) {
	if code, status, ok := mapValidationErrorToCode(reqErr); ok {
		return code, status
	}

	return mapNamedExceptionToCode(reqErr)
}

// mapValidationErrorToCode handles the generic "missing parameter" / "invalid
// parameter value" / "validation error" error families. See mapErrorToCode.
func mapValidationErrorToCode(reqErr error) (string, int, bool) {
	switch {
	case errors.Is(reqErr, ErrMissingRoleArn), errors.Is(reqErr, ErrMissingSessionName),
		errors.Is(reqErr, ErrMissingFederationTokenName), errors.Is(reqErr, ErrMissingWebIdentityToken),
		errors.Is(reqErr, ErrMissingSAMLAssertion), errors.Is(reqErr, ErrMissingPrincipalArn),
		errors.Is(reqErr, ErrMissingTargetPrincipal), errors.Is(reqErr, ErrMissingTaskPolicyArn),
		errors.Is(reqErr, ErrMissingTradeInToken), errors.Is(reqErr, ErrMissingAudience),
		errors.Is(reqErr, ErrMissingSigningAlgorithm), errors.Is(reqErr, ErrMFACodeRequired),
		errors.Is(reqErr, ErrMissingEncodedMessage):
		return "MissingParameter", http.StatusBadRequest, true
	case errors.Is(reqErr, ErrInvalidRoleArn), errors.Is(reqErr, ErrInvalidSourceIdentity),
		errors.Is(reqErr, ErrInvalidPrincipalArn), errors.Is(reqErr, ErrValidation):
		return invalidParamValue, http.StatusBadRequest, true
	case errors.Is(reqErr, ErrInvalidDuration), errors.Is(reqErr, ErrInvalidSessionName),
		errors.Is(reqErr, ErrInvalidFederationName), errors.Is(reqErr, ErrTooManyTags),
		errors.Is(reqErr, ErrTooManyAudiences), errors.Is(reqErr, ErrEmptyAccessKeyID),
		errors.Is(reqErr, ErrInvalidMFATokenCode), errors.Is(reqErr, ErrInvalidMFASerialNumber),
		errors.Is(reqErr, ErrInvalidTagKey), errors.Is(reqErr, ErrInvalidTagValue),
		errors.Is(reqErr, ErrTooManyPolicyArns), errors.Is(reqErr, ErrInvalidPolicyArn),
		errors.Is(reqErr, ErrInvalidProvidedContext), errors.Is(reqErr, ErrInvalidTargetPrincipal),
		errors.Is(reqErr, ErrTokenCodeWithoutSerial):
		return validationError, http.StatusBadRequest, true
	}

	return "", 0, false
}

// mapNamedExceptionToCode handles errors that map to a specific named AWS
// exception type rather than one of the generic validation buckets above.
// See mapErrorToCode.
func mapNamedExceptionToCode(reqErr error) (string, int) {
	switch {
	case errors.Is(reqErr, ErrMalformedPolicyDocument):
		return "MalformedPolicyDocument", http.StatusBadRequest
	case errors.Is(reqErr, ErrPackedPolicyTooLarge):
		return "PackedPolicyTooLarge", http.StatusBadRequest
	case errors.Is(reqErr, ErrExpiredToken), errors.Is(reqErr, ErrSessionExpired):
		return "ExpiredTokenException", http.StatusBadRequest
	case errors.Is(reqErr, ErrExpiredTradeInToken):
		return "ExpiredTradeInTokenException", http.StatusBadRequest
	case errors.Is(reqErr, ErrSessionDurationEscalation):
		return "SessionDurationEscalationException", http.StatusBadRequest
	case errors.Is(reqErr, ErrOutboundWebIdentityFederationDisabled):
		return "OutboundWebIdentityFederationDisabledException", http.StatusBadRequest
	case errors.Is(reqErr, ErrInvalidIdentityToken), errors.Is(reqErr, ErrInvalidSAMLAssertion):
		return "InvalidIdentityToken", http.StatusBadRequest
	case errors.Is(reqErr, ErrInvalidAuthorizationMessage):
		return "InvalidAuthorizationMessageException", http.StatusBadRequest
	case errors.Is(reqErr, ErrUnknownAccessKeyID):
		return "InvalidClientTokenId", http.StatusBadRequest
	case errors.Is(reqErr, ErrMissingAction), errors.Is(reqErr, ErrInvalidAction):
		return invalidAction, http.StatusBadRequest
	case errors.Is(reqErr, ErrIDPRejectedClaim), errors.Is(reqErr, ErrAccessDenied):
		return "AccessDenied", http.StatusForbidden
	}

	return "", 0
}

// handleError writes a standardised STS XML error response.
func (h *Handler) handleError(ctx context.Context, c *echo.Context, reqErr error) error {
	log := logger.Load(ctx)

	code := "InternalFailure"
	httpStatus := http.StatusInternalServerError

	if c, s := mapErrorToCode(reqErr); c != "" {
		code = c
		httpStatus = s
	}

	if httpStatus == http.StatusInternalServerError {
		log.ErrorContext(ctx, "STS internal error", "error", reqErr)
	} else {
		log.WarnContext(ctx, "STS request error", "error", reqErr)
	}

	errType := "Sender"
	if httpStatus == http.StatusInternalServerError {
		errType = "Receiver"
	}

	errResp := &ErrorResponse{
		Xmlns: STSNamespace,
		Error: ErrorDetail{
			Type:    errType,
			Code:    code,
			Message: reqErr.Error(),
		},
		RequestID: uuid.NewString(),
	}

	return writeXMLResponse(c, httpStatus, errResp)
}

// writeXMLResponse serialises payload to XML and writes it to the Echo response.
func writeXMLResponse(c *echo.Context, code int, payload any) error {
	var buf bytes.Buffer

	buf.WriteString(xml.Header)

	if err := xml.NewEncoder(&buf).Encode(payload); err != nil {
		return err
	}

	return c.Blob(code, "text/xml; charset=utf-8", buf.Bytes())
}

// parseFormValues parses URL-encoded form bytes into a simple key→value map.
func parseFormValues(body []byte) map[string]string {
	result := make(map[string]string)

	for pair := range strings.SplitSeq(string(body), "&") {
		if pair == "" {
			continue
		}

		kv := strings.SplitN(pair, "=", kvPairLen)
		if len(kv) != kvPairLen {
			continue
		}

		key, _ := url.QueryUnescape(kv[0])
		val, _ := url.QueryUnescape(kv[1])
		result[key] = val
	}

	return result
}

// parseSessionTags reads Tags.member.N.Key / Tags.member.N.Value form fields and
// returns them as a []Tag slice. It supports up to MaxTagCount+1 entries so that
// callers can detect and reject submissions that exceed the MaxTagCount limit.
func parseSessionTags(r *http.Request) []Tag {
	var tags []Tag

	for i := 1; i <= MaxTagCount+1; i++ {
		key := r.FormValue(fmt.Sprintf("Tags.member.%d.Key", i))
		if key == "" {
			break
		}

		value := r.FormValue(fmt.Sprintf("Tags.member.%d.Value", i))
		tags = append(tags, Tag{Key: key, Value: value})
	}

	return tags
}

// parseTransitiveTagKeys reads TransitiveTagKeys.member.N form fields.
func parseTransitiveTagKeys(r *http.Request) []string {
	var keys []string

	for i := 1; i <= MaxTagCount; i++ {
		key := r.FormValue(fmt.Sprintf("TransitiveTagKeys.member.%d", i))
		if key == "" {
			break
		}

		keys = append(keys, key)
	}

	return keys
}

// extractAccessKeyFromAuth parses the SigV4 Authorization header or query params and returns the access key ID.
func extractAccessKeyFromAuth(r *http.Request) string {
	return httputils.ExtractAccessKeyFromRequest(r)
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (h *Handler) Reset() {
	if b, ok := h.Backend.(*InMemoryBackend); ok {
		b.Reset()
	}
}

// SessionMetrics returns STS session and janitor sweep counters.
func (h *Handler) SessionMetrics() SessionMetrics {
	metrics := SessionMetrics{}

	if b, ok := h.Backend.(*InMemoryBackend); ok {
		metrics.ActiveSessions, metrics.ExpiredSessions = b.SessionCounts()
		metrics.TotalSessionsCreated = b.totalSessionsCreated.Load()
		metrics.OpsAssumeRole = b.cntAssumeRole.Load()
		metrics.OpsAssumeRoleWithSAML = b.cntAssumeRoleWithSAML.Load()
		metrics.OpsAssumeRoleWithWI = b.cntAssumeRoleWithWebIdentity.Load()
		metrics.OpsAssumeRoot = b.cntAssumeRoot.Load()
		metrics.OpsGetCallerIdentity = b.cntGetCallerIdentity.Load()
		metrics.OpsGetFederationToken = b.cntGetFederationToken.Load()
		metrics.OpsGetSessionToken = b.cntGetSessionToken.Load()
		metrics.OpsGetWebIdentityToken = b.cntGetWebIdentityToken.Load()
		metrics.OpsGetAccessKeyInfo = b.cntGetAccessKeyInfo.Load()
		metrics.OpsDecodeAuthMessage = b.cntDecodeAuthorizationMsg.Load()
		metrics.OpsGetDelegatedToken = b.cntGetDelegatedAccessToken.Load()
	}

	if h.janitor != nil {
		janitorMetrics := h.janitor.Metrics()
		metrics.SweepCount = janitorMetrics.SweepCount
		metrics.ExpiredEvictions = janitorMetrics.ExpiredEvictions
	}

	return metrics
}
