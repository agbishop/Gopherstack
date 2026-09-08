package organizations

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	orgService      = "organizations"
	orgTargetPrefix = "AWSOrganizationsV20161128."

	defaultMaxResults = 100
)

// Handler is the HTTP handler for the AWS Organizations JSON 1.1 API.
type Handler struct {
	Backend StorageBackend
}

// NewHandler creates a new Organizations handler.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{Backend: backend}
}

// Name returns the service name.
func (h *Handler) Name() string { return "Organizations" }

// GetSupportedOperations returns the list of supported Organizations operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"AcceptHandshake",
		"AttachPolicy",
		"CancelHandshake",
		"CloseAccount",
		"CreateAccount",
		"CreateGovCloudAccount",
		"CreateOrganization",
		"CreateOrganizationalUnit",
		"CreatePolicy",
		"DeclineHandshake",
		"DeleteOrganization",
		"DeleteOrganizationalUnit",
		"DeletePolicy",
		"DeleteResourcePolicy",
		"DeregisterDelegatedAdministrator",
		"DescribeAccount",
		"DescribeCreateAccountStatus",
		"DescribeEffectivePolicy",
		"DescribeHandshake",
		"DescribeOrganization",
		"DescribeOrganizationalUnit",
		"DescribePolicy",
		"DescribeResourcePolicy",
		"DescribeResponsibilityTransfer",
		"DetachPolicy",
		"DisableAWSServiceAccess",
		"DisablePolicyType",
		"EnableAWSServiceAccess",
		"EnableAllFeatures",
		"EnablePolicyType",
		"InviteAccountToOrganization",
		"InviteOrganizationToTransferResponsibility",
		"LeaveOrganization",
		"ListAccounts",
		"ListAccountsForParent",
		"ListAccountsWithInvalidEffectivePolicy",
		"ListAWSServiceAccessForOrganization",
		"ListChildren",
		"ListCreateAccountStatus",
		"ListDelegatedAdministrators",
		"ListDelegatedServicesForAccount",
		"ListEffectivePolicyValidationErrors",
		"ListHandshakesForAccount",
		"ListHandshakesForOrganization",
		"ListInboundResponsibilityTransfers",
		"ListOrganizationalUnitsForParent",
		"ListOutboundResponsibilityTransfers",
		"ListParents",
		"ListPolicies",
		"ListPoliciesForTarget",
		"ListRoots",
		"ListTagsForResource",
		"ListTargetsForPolicy",
		"MoveAccount",
		"PutResourcePolicy",
		"RegisterDelegatedAdministrator",
		"RemoveAccountFromOrganization",
		"TagResource",
		"TerminateResponsibilityTransfer",
		"UntagResource",
		"UpdateOrganizationalUnit",
		"UpdatePolicy",
		"UpdateResponsibilityTransfer",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return orgService }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// Reset clears all backend state (used in tests).
func (h *Handler) Reset() { h.Backend.Reset() }

// RouteMatcher returns a function that matches Organizations JSON 1.1 API requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), orgTargetPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// ExtractOperation extracts the operation name from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	return strings.TrimPrefix(c.Request().Header.Get("X-Amz-Target"), orgTargetPrefix)
}

// ExtractResource extracts the primary resource identifier from the request body.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var data map[string]any

	if uerr := json.Unmarshal(body, &data); uerr != nil {
		return ""
	}

	for _, key := range []string{"AccountId", "HandshakeId", "OrganizationalUnitId", "PolicyId", "ResourceId"} {
		if v, ok := data[key]; ok {
			if s, isStr := v.(string); isStr {
				return s
			}
		}
	}

	return ""
}

// Handler returns the Echo handler function for Organizations requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		target := c.Request().Header.Get("X-Amz-Target")

		if !strings.HasPrefix(target, orgTargetPrefix) {
			return h.writeError(c, http.StatusBadRequest, "InvalidParameterException",
				"missing or invalid X-Amz-Target header")
		}

		op := strings.TrimPrefix(target, orgTargetPrefix)

		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			log.ErrorContext(ctx, "organizations: failed to read request body", "error", err)

			return h.writeError(c, http.StatusInternalServerError, "InternalFailure", "failed to read request body")
		}

		log.DebugContext(ctx, "organizations request", "op", op)

		return h.dispatch(c, op, body)
	}
}

// dispatch routes to the appropriate handler based on the operation name.
func (h *Handler) dispatch(c *echo.Context, op string, body []byte) error {
	if ok, result := h.dispatchOrg(c, op, body); ok {
		return result
	}

	if ok, result := h.dispatchAccount(c, op, body); ok {
		return result
	}

	if ok, result := h.dispatchRoot(c, op, body); ok {
		return result
	}

	if ok, result := h.dispatchOU(c, op, body); ok {
		return result
	}

	if ok, result := h.dispatchPolicy(c, op, body); ok {
		return result
	}

	if ok, result := h.dispatchPolicyAttachments(c, op, body); ok {
		return result
	}

	if ok, result := h.dispatchTags(c, op, body); ok {
		return result
	}

	if ok, result := h.dispatchServiceAccess(c, op, body); ok {
		return result
	}

	if ok, result := h.dispatchDelegatedAdmin(c, op, body); ok {
		return result
	}

	if ok, result := h.dispatchHandshakeOps(c, op, body); ok {
		return result
	}

	if ok, result := h.dispatchTransferOps(c, op, body); ok {
		return result
	}

	if ok, result := h.dispatchResourcePolicy(c, op, body); ok {
		return result
	}

	if ok, result := h.dispatchEffectivePolicy(c, op, body); ok {
		return result
	}

	return h.writeError(c, http.StatusBadRequest, "UnknownOperationException", "unknown operation: "+op)
}

// ----------------------------------------
// Error handling
// ----------------------------------------

func (h *Handler) writeError(c *echo.Context, statusCode int, errType, message string) error {
	return c.JSON(statusCode, map[string]string{
		"__type":  errType,
		"message": message,
	})
}

const errConstraintViolation = "ConstraintViolationException"

const errInvalidInput = "InvalidInputException"

func getErrorTable() map[error]awserr.APIError {
	return map[error]awserr.APIError{
		ErrOrgNotFound:      {Code: "AWSOrganizationsNotInUseException", HTTPStatus: http.StatusBadRequest},
		ErrOrgAlreadyExists: {Code: "AlreadyInOrganizationException", HTTPStatus: http.StatusBadRequest},
		ErrAccountNotFound:  {Code: "AccountNotFoundException", HTTPStatus: http.StatusBadRequest},
		ErrOUNotFound: {
			Code:       "OrganizationalUnitNotFoundException",
			HTTPStatus: http.StatusBadRequest,
		},
		ErrPolicyNotFound:           {Code: "PolicyNotFoundException", HTTPStatus: http.StatusBadRequest},
		ErrPolicyTypeAlreadyEnabled: {Code: "PolicyTypeAlreadyEnabledException", HTTPStatus: http.StatusBadRequest},
		ErrPolicyTypeNotEnabled:     {Code: "PolicyTypeNotEnabledException", HTTPStatus: http.StatusBadRequest},
		ErrCreateAccountStatusNotFound: {
			Code:       "CreateAccountStatusNotFoundException",
			HTTPStatus: http.StatusBadRequest,
		},
		ErrDuplicatePolicyAttachment: {
			Code:       "DuplicatePolicyAttachmentException",
			HTTPStatus: http.StatusBadRequest,
		},
		ErrPolicyNotAttached:           {Code: "PolicyNotAttachedException", HTTPStatus: http.StatusBadRequest},
		ErrInvalidInput:                {Code: errInvalidInput, HTTPStatus: http.StatusBadRequest},
		ErrChildNotFound:               {Code: "ChildNotFoundException", HTTPStatus: http.StatusBadRequest},
		ErrDelegatedAdminNotFound:      {Code: "AccountNotRegisteredException", HTTPStatus: http.StatusBadRequest},
		ErrDelegatedAdminAlreadyExists: {Code: "AccountAlreadyRegisteredException", HTTPStatus: http.StatusBadRequest},
		ErrPolicyLimitExceeded:         {Code: errConstraintViolation, HTTPStatus: http.StatusBadRequest},
		ErrHandshakeNotFound:           {Code: "HandshakeNotFoundException", HTTPStatus: http.StatusBadRequest},
		ErrHandshakeConstraintViolation: {
			Code:       "HandshakeConstraintViolationException",
			HTTPStatus: http.StatusBadRequest,
		},
		ErrResourcePolicyNotFound:  {Code: "ResourcePolicyNotFoundException", HTTPStatus: http.StatusBadRequest},
		ErrEffectivePolicyNotFound: {Code: "EffectivePolicyNotFoundException", HTTPStatus: http.StatusBadRequest},
		ErrAccountAlreadyClosed:    {Code: errConstraintViolation, HTTPStatus: http.StatusBadRequest},
		ErrOUDepthLimitExceeded:    {Code: errConstraintViolation, HTTPStatus: http.StatusBadRequest},
		ErrDuplicateOrganizationalUnit: {
			Code:       "DuplicateOrganizationalUnitException",
			HTTPStatus: http.StatusBadRequest,
		},
		ErrTargetNotFound:       {Code: "TargetNotFoundException", HTTPStatus: http.StatusBadRequest},
		ErrServiceNotEnabled:    {Code: errConstraintViolation, HTTPStatus: http.StatusBadRequest},
		ErrPolicyInUse:          {Code: "PolicyInUseException", HTTPStatus: http.StatusBadRequest},
		ErrOrganizationNotEmpty: {Code: "OrganizationNotEmptyException", HTTPStatus: http.StatusBadRequest},
		ErrDuplicateHandshake:   {Code: "DuplicateHandshakeException", HTTPStatus: http.StatusBadRequest},
		ErrPolicyTypeAttached:   {Code: errConstraintViolation, HTTPStatus: http.StatusBadRequest},
		ErrMalformedPolicyDocument: {
			Code:       "MalformedPolicyDocumentException",
			HTTPStatus: http.StatusBadRequest,
		},
		ErrPolicyContentLimitExceeded: {Code: errConstraintViolation, HTTPStatus: http.StatusBadRequest},
		ErrTagLimitExceeded:           {Code: errConstraintViolation, HTTPStatus: http.StatusBadRequest},
		ErrInvalidSystemTags:          {Code: errInvalidInput, HTTPStatus: http.StatusBadRequest},
		ErrDuplicateTagKey:            {Code: errInvalidInput, HTTPStatus: http.StatusBadRequest},
		ErrInvalidTagKeyLength:        {Code: errInvalidInput, HTTPStatus: http.StatusBadRequest},
		ErrInvalidTagValueLength:      {Code: errInvalidInput, HTTPStatus: http.StatusBadRequest},
		ErrResponsibilityTransferNotFound: {
			Code:       "ResponsibilityTransferNotFoundException",
			HTTPStatus: http.StatusBadRequest,
		},
		ErrInvalidResponsibilityTransferTransition: {
			Code:       "InvalidResponsibilityTransferTransitionException",
			HTTPStatus: http.StatusBadRequest,
		},
		ErrResponsibilityTransferAlreadyInStatus: {
			Code:       "ResponsibilityTransferAlreadyInStatusException",
			HTTPStatus: http.StatusBadRequest,
		},
		ErrOrganizationalUnitNotEmpty: {
			Code:       "OrganizationalUnitNotEmptyException",
			HTTPStatus: http.StatusBadRequest,
		},
		ErrMasterCannotLeaveOrganization: {
			Code:       "MasterCannotLeaveOrganizationException",
			HTTPStatus: http.StatusBadRequest,
		},
		ErrSourceParentNotFound: {
			Code:       "SourceParentNotFoundException",
			HTTPStatus: http.StatusBadRequest,
		},
		ErrDestinationParentNotFound: {
			Code:       "DestinationParentNotFoundException",
			HTTPStatus: http.StatusBadRequest,
		},
		ErrCannotRemoveDelegatedAdministratorFromOrg: {
			Code:       errConstraintViolation,
			HTTPStatus: http.StatusBadRequest,
		},
		ErrAccessDeniedManagedPolicy: {Code: "AccessDeniedException", HTTPStatus: http.StatusBadRequest},
	}
}

func (h *Handler) handleBackendError(c *echo.Context, err error) error {
	apiErr := awserr.Classify(err, getErrorTable(), awserr.APIError{
		Code:       "InternalFailure",
		Message:    err.Error(),
		HTTPStatus: http.StatusInternalServerError,
	})

	msg := apiErr.Message
	if idx := strings.Index(msg, ":"); idx > 0 {
		msg = strings.TrimSpace(msg[idx+1:])
	}

	return h.writeError(c, apiErr.HTTPStatus, apiErr.Code, msg)
}
