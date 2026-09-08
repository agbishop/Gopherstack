package sns

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// errCodeInvalidParameter is the SNS wire code for types.InvalidParameterException.
const errCodeInvalidParameter = "InvalidParameter"

// writeXML marshals v to XML and writes an HTTP 200 OK response.
func (h *Handler) writeXML(c *echo.Context, v any) error {
	httputils.WriteXML(c.Request().Context(), c.Response(), http.StatusOK, v)

	return nil
}

// writeError writes an XML error response.
func (h *Handler) writeError(c *echo.Context, status int, code, message string) error {
	errResp := ErrorResponse{
		Error:     Error{Type: "Sender", Code: code, Message: message},
		RequestID: uuid.NewString(),
	}

	httputils.WriteXML(c.Request().Context(), c.Response(), status, errResp)

	return nil
}

// handleBackendError maps a backend error to an XML error response.
func (h *Handler) handleBackendError(c *echo.Context, err error) error {
	ctx := c.Request().Context()
	log := logger.Load(ctx)

	code := errorCode(err)
	status := http.StatusBadRequest

	switch {
	case errors.Is(err, ErrTopicNotFound), errors.Is(err, ErrSubscriptionNotFound),
		errors.Is(err, ErrPlatformApplicationNotFound), errors.Is(err, ErrEndpointNotFound),
		errors.Is(err, ErrPhoneNumberNotFound):
		log.WarnContext(ctx, "SNS resource not found", "error", err)
	case errors.Is(err, ErrTopicAlreadyExists), errors.Is(err, ErrPlatformApplicationAlreadyExists),
		errors.Is(err, ErrSandboxPhoneAlreadyExists):
		log.WarnContext(ctx, "SNS resource already exists", "error", err)
	case errors.Is(err, ErrInvalidParameter), errors.Is(err, ErrSandboxPhoneNotVerified):
		log.WarnContext(ctx, "SNS invalid parameter", "error", err)
	case errors.Is(err, ErrEndpointDisabled):
		log.WarnContext(ctx, "SNS endpoint disabled", "error", err)
	case errors.Is(err, ErrOptedOut):
		log.WarnContext(ctx, "SNS phone number opted out", "error", err)
	case errors.Is(err, ErrPermissionLabelExists), errors.Is(err, ErrPermissionLabelNotFound):
		status = http.StatusForbidden
		log.WarnContext(ctx, "SNS permission label error", "error", err)
	case errors.Is(err, ErrSubscriptionLimitExceeded), errors.Is(err, ErrFilterPolicyLimitExceeded):
		status = http.StatusForbidden
		log.WarnContext(ctx, "SNS limit exceeded", "error", err)
	default:
		status = http.StatusInternalServerError
		log.ErrorContext(ctx, "SNS internal error", "error", err)
	}

	return h.writeError(c, status, code, err.Error())
}

// errorCode returns the SNS error code string for the given error.
func errorCode(err error) string {
	switch {
	case errors.Is(err, ErrTopicNotFound), errors.Is(err, ErrSubscriptionNotFound),
		errors.Is(err, ErrPlatformApplicationNotFound), errors.Is(err, ErrEndpointNotFound):
		return "NotFound"
	case errors.Is(err, ErrPhoneNumberNotFound):
		// DeleteSMSSandboxPhoneNumber and VerifySMSSandboxPhoneNumber both
		// declare "ResourceNotFound" (not "NotFound") in their own
		// deserializeOpError<Op> switch -- matches this sentinel's own
		// message text, unlike the shared "NotFound" bucket above.
		return "ResourceNotFound"
	case errors.Is(err, ErrTopicAlreadyExists):
		return "TopicAlreadyExists"
	case errors.Is(err, ErrPlatformApplicationAlreadyExists):
		// CreatePlatformApplication's own deserializeOpError models only
		// AuthorizationError, InternalError, InvalidParameter -- no
		// "already exists" shape exists in the pinned SNS module.
		return errCodeInvalidParameter
	case errors.Is(err, ErrSandboxPhoneAlreadyExists):
		// CreateSMSSandboxPhoneNumber's own deserializeOpError models no
		// "already exists" shape either; UserError ("a request parameter
		// does not comply with the associated constraints") is the nearest
		// modelled fit for the uniqueness violation. UNCONFIRMED against
		// AWS prose docs -- see PARITY.md.
		return "UserError"
	case errors.Is(err, ErrInvalidParameter), errors.Is(err, ErrSandboxPhoneNotVerified):
		return errCodeInvalidParameter
	case errors.Is(err, ErrEndpointDisabled):
		return "EndpointDisabled"
	case errors.Is(err, ErrOptedOut):
		// Publish is the only call site (PublishSMS), and
		// awsAwsquery_deserializeOpErrorPublish (sns@v1.42.4 deserializers.go)
		// models no "OptedOut" shape -- OptedOutException exists only on
		// CreateSMSSandboxPhoneNumber's switch. InvalidParameter is the same
		// nearest-fit precedent used above for ErrPlatformApplicationAlreadyExists.
		return errCodeInvalidParameter
	case errors.Is(err, ErrPermissionLabelExists), errors.Is(err, ErrPermissionLabelNotFound):
		return "AuthorizationError"
	case errors.Is(err, ErrSubscriptionLimitExceeded):
		return "SubscriptionLimitExceeded"
	case errors.Is(err, ErrFilterPolicyLimitExceeded):
		return "FilterPolicyLimitExceeded"
	default:
		return "InternalError"
	}
}
