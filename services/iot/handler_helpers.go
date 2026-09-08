package iot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"
)

// errTypeInvalidRequest is the AWS IoT restjson1 __type for a malformed or
// otherwise invalid request body -- shared across every call site that
// rejects a request before it reaches backend validation.
const errTypeInvalidRequest = "InvalidRequestException"

func readBody(c *echo.Context, dst any) error {
	if err := json.NewDecoder(c.Request().Body).Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		if jsonErr := c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()}); jsonErr != nil {
			return jsonErr
		}

		return err
	}

	return nil
}

type awsErrBody struct {
	Type    string `json:"__type"`
	Message string `json:"message"`
}

func respondNotFound(c *echo.Context, msg string) error {
	return c.JSON(http.StatusNotFound, awsErrBody{"ResourceNotFoundException", msg})
}

func respondConflict(c *echo.Context, msg string) error {
	return c.JSON(http.StatusConflict, awsErrBody{"ResourceAlreadyExistsException", msg})
}

// writeIoTError maps a backend sentinel error to the AWS IoT restjson1 error
// shape ({"__type", "message"}) and HTTP status. Single source of truth
// shared by respondErr and Handler.handleError, so every handler gets the
// same ResourceNotFoundException/InvalidRequestException/
// ResourceAlreadyExistsException/VersionConflictException/
// DeleteConflictException/VersionsLimitExceededException/
// InvalidStateTransitionException mapping.
func writeIoTError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrThingNotFound),
		errors.Is(err, ErrPolicyNotFound),
		errors.Is(err, ErrThingTypeNotFound),
		errors.Is(err, ErrThingGroupNotFound),
		errors.Is(err, ErrCertificateNotFound),
		errors.Is(err, ErrCertificateProviderNotFound),
		errors.Is(err, ErrPolicyVersionNotFound),
		errors.Is(err, ErrRegistrationTaskNotFound),
		errors.Is(err, ErrManagedJobTemplateNotFound),
		errors.Is(err, ErrIndexNotFound),
		errors.Is(err, ErrShadowNotFound),
		errors.Is(err, ErrResourceNotFound):

		return respondNotFound(c, err.Error())
	// ErrRuleNotFound/ErrTopicRuleDestinationNotFound are deliberately NOT
	// grouped above: none of GetTopicRule/DeleteTopicRule/DisableTopicRule/
	// EnableTopicRule/ReplaceTopicRule/GetTopicRuleDestination/
	// UpdateTopicRuleDestination/DeleteTopicRuleDestination's own
	// deserializeOpError switches (iot@v1.77.4/deserializers.go) declare a
	// ResourceNotFoundException case -- this family's real vocabulary has
	// no not-found type at all; InvalidRequestException is the only
	// declared client-fault type available.
	case errors.Is(err, ErrValidation),
		errors.Is(err, ErrRuleNotFound),
		errors.Is(err, ErrTopicRuleDestinationNotFound):

		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	case errors.Is(err, ErrAlreadyExists):

		return respondConflict(c, err.Error())
	case errors.Is(err, ErrVersionConflict):

		return c.JSON(http.StatusConflict, awsErrBody{"VersionConflictException", err.Error()})
	case errors.Is(err, ErrDeleteConflict):

		return c.JSON(http.StatusConflict, awsErrBody{"DeleteConflictException", err.Error()})
	case errors.Is(err, ErrVersionsLimitExceeded):

		return c.JSON(http.StatusConflict, awsErrBody{"VersionsLimitExceededException", err.Error()})
	case errors.Is(err, ErrInvalidStateTransition):

		return c.JSON(http.StatusConflict, awsErrBody{"InvalidStateTransitionException", err.Error()})
	default:

		return c.JSON(http.StatusInternalServerError, awsErrBody{"InternalFailureException", err.Error()})
	}
}

func respondErr(c *echo.Context, err error) error {
	return writeIoTError(c, err)
}

// respondAsInvalidRequest renders err as InvalidRequestException (400) when
// it wraps sentinel, falling through to the shared writeIoTError mapping
// otherwise. Several operations' own deserializeOpError switches (per-op,
// read directly from iot@v1.77.4/deserializers.go) declare no
// ResourceNotFoundException/DeleteConflictException/
// InvalidStateTransitionException case at all even though the backend
// signals the condition via the generic ErrResourceNotFound/
// ErrThingGroupNotFound/ErrDeleteConflict/ErrInvalidStateTransition
// sentinels those helpers use for operations elsewhere that DO declare the
// richer type -- InvalidRequestException is the only client-fault type
// those specific operations declare. Kept as a per-call-site override
// rather than a change to writeIoTError's own mapping because the same
// sentinels are shared by other operations that genuinely need the richer
// type.
func respondAsInvalidRequest(c *echo.Context, err, sentinel error) error {
	if errors.Is(err, sentinel) {
		return c.JSON(http.StatusBadRequest, awsErrBody{errTypeInvalidRequest, err.Error()})
	}

	return writeIoTError(c, err)
}

// respondAsConflictCode renders err as the given AWS wire code (HTTP 409)
// when it wraps sentinel, falling through to the shared writeIoTError
// mapping otherwise. Same rationale as respondAsInvalidRequest: a
// per-call-site override, because ErrAlreadyExists (writeIoTError's
// default: ResourceAlreadyExistsException) is shared with operations
// elsewhere that genuinely declare that code -- CreateCommand/
// CreatePackage/CreatePackageVersion/CreateJobTemplate declare
// ConflictException instead (iot@v1.77.4 deserializers.go, confirmed
// per-op), and StartAuditMitigationActionsTask/
// StartDetectMitigationActionsTask declare TaskAlreadyExistsException, a
// code writeIoTError has never rendered.
func respondAsConflictCode(c *echo.Context, err, sentinel error, code string) error {
	if errors.Is(err, sentinel) {
		return c.JSON(http.StatusConflict, awsErrBody{code, err.Error()})
	}

	return writeIoTError(c, err)
}

func parseInt32(s string, out *int32) error {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return err
	}
	*out = int32(n) //nolint:gosec // safe: versionId values are small positive integers

	return nil
}

// parseInt32QueryParam reads an int32 query parameter, defaulting to 0 when
// absent or invalid.
func parseInt32QueryParam(c *echo.Context, name string) int32 {
	v := c.QueryParam(name)
	if v == "" {
		return 0
	}

	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return 0
	}

	return int32(n)
}

// parseExpectedVersionQueryParam reads the `expectedVersion` query param,
// defaulting to 0 (unset) when absent or invalid. Several Delete* ops
// declare it (e.g. awsRestjson1_serializeOpHttpBindingsDeleteThingInput,
// iot@v1.77.4/serializers.go) -- 0 is a safe "not specified" sentinel since
// AWS IoT resource versions start at 1.
func parseExpectedVersionQueryParam(c *echo.Context) int64 {
	v := c.QueryParam("expectedVersion")
	if v == "" {
		return 0
	}

	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0
	}

	return n
}
