package acmpca

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	svcTags "github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	acmpcaMatchPriority = 82
	acmpcaTargetPrefix  = "ACMPrivateCA."
	daysPerYear         = 365
	daysPerMonth        = 30
	hoursPerDay         = 24
)

// Handler is the Echo HTTP handler for ACM PCA operations.
type Handler struct {
	Backend *InMemoryBackend
	tags    map[string]*svcTags.Tags
	tagsMu  *lockmetrics.RWMutex
}

// NewHandler creates a new ACM PCA handler.
func NewHandler(backend *InMemoryBackend) *Handler {
	return &Handler{
		Backend: backend,
		tags:    make(map[string]*svcTags.Tags),
		tagsMu:  lockmetrics.New("acmpca.tags"),
	}
}

// Name returns the service name.
func (h *Handler) Name() string { return "ACMPCA" }

// GetSupportedOperations returns the list of supported ACM PCA operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"CreateCertificateAuthority",
		"CreateCertificateAuthorityAuditReport",
		"CreatePermission",
		"DeleteCertificateAuthority",
		"DeletePermission",
		"DeletePolicy",
		"DescribeCertificateAuthority",
		"DescribeCertificateAuthorityAuditReport",
		"GetCertificate",
		"GetCertificateAuthorityCertificate",
		"GetCertificateAuthorityCsr",
		"GetPolicy",
		"ImportCertificateAuthorityCertificate",
		"IssueCertificate",
		"ListCertificateAuthorities",
		"ListPermissions",
		"ListTags",
		"PutPolicy",
		"RevokeCertificate",
		"RestoreCertificateAuthority",
		"TagCertificateAuthority",
		"UntagCertificateAuthority",
		"UpdateCertificateAuthority",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "acm-pca" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this ACM PCA instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches ACM PCA JSON-protocol requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		r := c.Request()
		if r.Method != http.MethodPost {
			return false
		}

		if strings.HasPrefix(r.URL.Path, "/dashboard/") {
			return false
		}

		target := r.Header.Get("X-Amz-Target")

		return strings.HasPrefix(target, acmpcaTargetPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return acmpcaMatchPriority }

// ExtractOperation extracts the ACM PCA action from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")

	return strings.TrimPrefix(target, acmpcaTargetPrefix)
}

// ExtractResource returns the primary ARN from the JSON body.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var m map[string]json.RawMessage
	if unmarshalErr := json.Unmarshal(body, &m); unmarshalErr != nil {
		return ""
	}

	return extractFirstResourceByKeys(m, "CertificateAuthorityArn", "CertificateArn", "ResourceArn")
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		region := httputils.ExtractRegionFromRequest(c.Request(), h.Backend.Region())
		ctx := context.WithValue(c.Request().Context(), regionContextKey{}, region)

		return service.HandleTarget(
			c, logger.Load(ctx),
			"ACMPCA", "application/x-amz-json-1.1",
			h.GetSupportedOperations(),
			func(_ context.Context, action string, body []byte) ([]byte, error) {
				return h.dispatch(ctx, action, body)
			},
			h.handleError,
		)
	}
}

// dispatch routes the operation to the appropriate handler and marshals the response.
func (h *Handler) dispatch(ctx context.Context, action string, body []byte) ([]byte, error) {
	resp, err := h.dispatchJSON(ctx, action, body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(resp)
}

// handleError writes a standardized error response back to the client.
func (h *Handler) handleError(_ context.Context, c *echo.Context, action string, reqErr error) error {
	if errors.Is(reqErr, errUnknownACMPCAAction) {
		return h.writeJSONError(c, http.StatusBadRequest, "InvalidAction",
			action+" is not a valid ACM PCA action")
	}

	return h.handleOpError(c, action, reqErr)
}

var errUnknownACMPCAAction = errors.New("unknown ACM PCA action")

// ---- dispatch ----

func (h *Handler) dispatchJSON(ctx context.Context, action string, body []byte) (any, error) {
	switch action {
	case "CreateCertificateAuthority":
		return h.jsonCreateCA(ctx, body)
	case "DescribeCertificateAuthority":
		return h.jsonDescribeCA(ctx, body)
	case "ListCertificateAuthorities":
		return h.jsonListCAs(ctx, body)
	case "DeleteCertificateAuthority":
		return h.jsonDeleteCA(ctx, body)
	case "UpdateCertificateAuthority":
		return h.jsonUpdateCA(ctx, body)
	case "GetCertificateAuthorityCsr":
		return h.jsonGetCsr(ctx, body)
	case "ImportCertificateAuthorityCertificate":
		return h.jsonImportCACert(ctx, body)
	case "GetCertificateAuthorityCertificate":
		return h.jsonGetCACert(ctx, body)
	default:
		return h.dispatchCertAndTagOps(ctx, action, body)
	}
}

func (h *Handler) dispatchCertAndTagOps(ctx context.Context, action string, body []byte) (any, error) {
	switch action {
	case "IssueCertificate":
		return h.jsonIssueCert(ctx, body)
	case "GetCertificate":
		return h.jsonGetCert(ctx, body)
	case "RevokeCertificate":
		return h.jsonRevokeCert(ctx, body)
	case "ListPermissions":
		return h.jsonListPermissions(ctx, body)
	case "TagCertificateAuthority":
		return h.jsonTagCA(ctx, body)
	case "UntagCertificateAuthority":
		return h.jsonUntagCA(ctx, body)
	case "ListTags":
		return h.jsonListTags(ctx, body)
	default:
		return h.dispatchPermissionAndAuditOps(ctx, action, body)
	}
}

func (h *Handler) dispatchPermissionAndAuditOps(ctx context.Context, action string, body []byte) (any, error) {
	switch action {
	case "CreateCertificateAuthorityAuditReport":
		return h.jsonCreateAuditReport(ctx, body)
	case "CreatePermission":
		return h.jsonCreatePermission(ctx, body)
	case "DeletePermission":
		return h.jsonDeletePermission(ctx, body)
	case "DeletePolicy":
		return h.jsonDeletePolicy(ctx, body)
	case "DescribeCertificateAuthorityAuditReport":
		return h.jsonDescribeAuditReport(ctx, body)
	case "GetPolicy":
		return h.jsonGetPolicy(ctx, body)
	case "PutPolicy":
		return h.jsonPutPolicy(ctx, body)
	case "RestoreCertificateAuthority":
		return h.jsonRestoreCA(ctx, body)
	default:
		return nil, errUnknownACMPCAAction
	}
}

// ---- error handling ----

func (h *Handler) handleOpError(c *echo.Context, action string, opErr error) error {
	statusCode := http.StatusBadRequest
	var code string

	switch {
	case errors.Is(opErr, ErrCANotFound), errors.Is(opErr, ErrCertNotFound),
		errors.Is(opErr, ErrPermissionNotFound), errors.Is(opErr, ErrPolicyNotFound),
		errors.Is(opErr, ErrAuditReportNotFound):
		code = "ResourceNotFoundException"
	case errors.Is(opErr, ErrInvalidArgs):
		code = "InvalidArgsException"
	case errors.Is(opErr, ErrInvalidArn):
		code = "InvalidArnException"
	case errors.Is(opErr, ErrInvalidRequest):
		code = "InvalidRequestException"
	case errors.Is(opErr, ErrInvalidPolicy):
		code = "InvalidPolicyException"
	case errors.Is(opErr, ErrMalformedCertificate):
		code = "MalformedCertificateException"
	case errors.Is(opErr, ErrMalformedCSR):
		code = "MalformedCSRException"
	case errors.Is(opErr, ErrInvalidState):
		code = "InvalidStateException"
	case errors.Is(opErr, ErrPermissionAlreadyExists):
		code = "PermissionAlreadyExistsException"
	case errors.Is(opErr, ErrTooManyTags):
		code = "TooManyTagsException"
	case errors.Is(opErr, ErrRequestAlreadyProcessed):
		code = "RequestAlreadyProcessedException"
	default:
		code = "InternalFailure"
		statusCode = http.StatusInternalServerError
		logger.Load(c.Request().Context()).Error("ACM PCA internal error", "error", opErr, "action", action)
	}

	return h.writeJSONError(c, statusCode, code, opErr.Error())
}

func (h *Handler) writeJSONError(c *echo.Context, statusCode int, code, message string) error {
	return c.JSON(statusCode, map[string]string{
		"__type":  code,
		"message": message,
	})
}

// ---- helpers ----

// decodeBase64Field decodes a base64-encoded blob field. aws-sdk-go-v2 declares
// Csr (IssueCertificate) and Certificate/CertificateChain
// (ImportCertificateAuthorityCertificate) as Go []byte, which the awsjson1.1
// serializer base64-encodes on the wire (see serializers.go
// awsAwsjson11_serializeOpDocumentIssueCertificateInput /
// ...ImportCertificateAuthorityCertificateInput: both call Base64EncodeBytes).
// Using the JSON string as-is here would hand raw base64 text to pem.Decode and
// always fail for real SDK clients.
func decodeBase64Field(encoded, fieldName string, sentinel error) (string, error) {
	if encoded == "" {
		return "", nil
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: %s must be base64-encoded: %w", sentinel, fieldName, err)
	}

	return string(decoded), nil
}

// extractFirstResourceByKeys returns the first string resource found for the provided JSON field names.
func extractFirstResourceByKeys(body map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		if raw, ok := body[key]; ok {
			var resourceID string
			if err := json.Unmarshal(raw, &resourceID); err == nil {
				return resourceID
			}
		}
	}

	return ""
}

// Reset clears all handler tag state and delegates to the backend Reset.
func (h *Handler) Reset() {
	func() {
		h.tagsMu.Lock("Reset")
		defer h.tagsMu.Unlock()

		for _, t := range h.tags {
			t.Close()
		}
		h.tags = make(map[string]*svcTags.Tags)
	}()

	h.Backend.Reset()
}
