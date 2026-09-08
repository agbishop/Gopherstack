package cognitoidp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	errInvalidParameterException = "InvalidParameterException"
	keyDeliveryMedium            = "DeliveryMedium"
	keyDestination               = "Destination"
	keyAttributeName             = "AttributeName"
	mockDestination              = "mock"
	keyConfirmationCode          = "ConfirmationCode"
	// keyCustomMessage/keyCustomMessageSubject are gopherstack extensions (not part
	// of the real AWS CodeDeliveryDetailsType) that surface a CustomMessage Lambda
	// trigger's returned emailMessage/emailSubject (or smsMessage) so integration
	// tests can observe the trigger fired and read its output, mirroring the
	// existing keyConfirmationCode convenience above.
	keyCustomMessage         = "CustomMessage"
	keyCustomMessageSubject  = "CustomMessageSubject"
	authTypeBearer           = "Bearer"
	authFlowRefreshToken     = "REFRESH_TOKEN"
	authFlowRefreshTokenAuth = "REFRESH_TOKEN_AUTH"
	authFlowUserSRP          = "USER_SRP_AUTH"
	authFlowAdminUserSRP     = "ADMIN_USER_SRP_AUTH"
)

const (
	cognitoTargetPrefix = "AWSCognitoIdentityProviderService."
	jwksPathSuffix      = "/.well-known/jwks.json"
	contentType         = "application/x-amz-json-1.1"
)

var errUnknownAction = errors.New("UnknownOperationException")

// Handler is the Echo HTTP handler for Cognito IDP operations.
type Handler struct {
	Backend *InMemoryBackend `json:"backend,omitempty"`
	janitor *Janitor
	ops     map[string]service.JSONOpFunc
	region  string
}

// NewHandler creates a new Cognito IDP handler.
func NewHandler(backend *InMemoryBackend, region string) *Handler {
	h := &Handler{Backend: backend, region: region}
	h.ops = h.dispatchTable()

	return h
}

// WithJanitor attaches a background janitor to the handler.
// The janitor periodically evicts expired refresh tokens. interval=0 uses the default.
// The optional taskTimeout bounds each sweep; 0 means no per-task timeout.
func (h *Handler) WithJanitor(interval time.Duration, taskTimeout ...time.Duration) *Handler {
	j := NewJanitor(h.Backend, interval)
	if len(taskTimeout) > 0 {
		j.TaskTimeout = taskTimeout[0]
	}

	h.janitor = j

	return h
}

// StartWorker starts the background janitor if it is configured.
func (h *Handler) StartWorker(ctx context.Context) error {
	if h.janitor != nil {
		go h.janitor.Run(ctx)
	}

	return nil
}

// Reset clears all backend state. Useful for test isolation.
func (h *Handler) Reset() { h.Backend.Reset() }

// Name returns the service name.
func (h *Handler) Name() string { return "CognitoIDP" }

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	ops := baseSupportedOperations()
	ops = append(ops, extendedSupportedOperations()...)

	return ops
}

// baseSupportedOperations returns the original day-one Cognito IDP operation set.
func baseSupportedOperations() []string {
	return []string{
		"CreateUserPool",
		"DescribeUserPool",
		"ListUserPools",
		"DeleteUserPool",
		"UpdateUserPool",
		"GetUserPoolMfaConfig",
		"CreateUserPoolClient",
		"DescribeUserPoolClient",
		"ListUserPoolClients",
		"DeleteUserPoolClient",
		"UpdateUserPoolClient",
		"SignUp",
		"ConfirmSignUp",
		"InitiateAuth",
		"AdminInitiateAuth",
		"AdminCreateUser",
		"AdminSetUserPassword",
		"AdminGetUser",
		"AdminConfirmSignUp",
		"AdminDeleteUser",
		"AdminResetUserPassword",
		"ListUsers",
		"ForgotPassword",
		"ConfirmForgotPassword",
		"GetUser",
		"ChangePassword",
		"DeleteUser",
		"DeleteUserAttributes",
		"VerifyUserAttribute",
		"CreateGroup",
		"DeleteGroup",
		"GetGroup",
		"ListGroups",
		"AdminAddUserToGroup",
		"AdminRemoveUserFromGroup",
		"AdminListGroupsForUser",
		"UpdateUserAttributes",
		"AdminUpdateUserAttributes",
		"RevokeToken",
		"AddCustomAttributes",
		"AddUserPoolClientSecret",
		"AdminDeleteUserAttributes",
		"AdminDisableProviderForUser",
		"AdminDisableUser",
		"AdminEnableUser",
		"AdminForgetDevice",
		"ListUsersInGroup",
		"AdminUserGlobalSignOut",
		"GlobalSignOut",
		"ResendConfirmationCode",
		"SetUserPoolMfaConfig",
		"UpdateGroup",
		"GetSigningCertificate",
	}
}

// extendedSupportedOperations returns operations added in later completeness/accuracy passes.
func extendedSupportedOperations() []string {
	return []string{
		// parity-4 pass — new SDK operations (user pool replicas, admin auth
		// factors, provisioned limits): see user_pool_replicas.go,
		// provisioned_limits.go, and AdminGetUserAuthFactors in users.go.
		"AdminGetUserAuthFactors",
		"CreateUserPoolReplica",
		"DeleteUserPoolReplica",
		"GetProvisionedLimit",
		"ListUserPoolReplicas",
		"UpdateProvisionedLimit",
		"UpdateUserPoolReplica",
		// Completeness pass — previously notImplemented
		"AdminGetDevice",
		"AdminLinkProviderForUser",
		"AdminListDevices",
		"AdminListUserAuthEvents",
		"AdminRespondToAuthChallenge",
		"AdminSetUserMFAPreference",
		"AdminSetUserSettings",
		"AdminUpdateAuthEventFeedback",
		"AdminUpdateDeviceStatus",
		"AssociateSoftwareToken",
		"CompleteWebAuthnRegistration",
		"ConfirmDevice",
		"CreateIdentityProvider",
		"CreateManagedLoginBranding",
		"CreateResourceServer",
		"CreateTerms",
		"CreateUserImportJob",
		"CreateUserPoolDomain",
		"DeleteIdentityProvider",
		"DeleteManagedLoginBranding",
		"DeleteResourceServer",
		"DeleteTerms",
		"DeleteUserPoolClientSecret",
		"DeleteUserPoolDomain",
		"DeleteWebAuthnCredential",
		"DescribeIdentityProvider",
		"DescribeManagedLoginBranding",
		"DescribeManagedLoginBrandingByClient",
		"DescribeResourceServer",
		"DescribeRiskConfiguration",
		"DescribeTerms",
		"DescribeUserImportJob",
		"DescribeUserPoolDomain",
		"ForgetDevice",
		"GetCSVHeader",
		"GetDevice",
		"GetIdentityProviderByIdentifier",
		"GetLogDeliveryConfiguration",
		"GetTokensFromRefreshToken",
		"GetUICustomization",
		"GetUserAttributeVerificationCode",
		"GetUserAuthFactors",
		"ListDevices",
		"ListIdentityProviders",
		"ListResourceServers",
		"ListTagsForResource",
		"ListTerms",
		"ListUserImportJobs",
		"ListUserPoolClientSecrets",
		"ListWebAuthnCredentials",
		"RespondToAuthChallenge",
		"SetLogDeliveryConfiguration",
		"SetRiskConfiguration",
		"SetUICustomization",
		"SetUserMFAPreference",
		"SetUserSettings",
		"StartUserImportJob",
		"StartWebAuthnRegistration",
		"StopUserImportJob",
		"TagResource",
		"UntagResource",
		"UpdateAuthEventFeedback",
		"UpdateDeviceStatus",
		"UpdateIdentityProvider",
		"UpdateManagedLoginBranding",
		"UpdateResourceServer",
		"UpdateTerms",
		"UpdateUserPoolDomain",
		"VerifySoftwareToken",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "cognito-idp" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this Cognito IDP instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.region} }

// RouteMatcher returns a function that matches Cognito IDP requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		if strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), cognitoTargetPrefix) {
			return true
		}

		return strings.HasSuffix(c.Request().URL.Path, jwksPathSuffix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// ExtractOperation extracts the Cognito action from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	action := strings.TrimPrefix(target, cognitoTargetPrefix)

	if action == "" || action == target {
		if strings.HasSuffix(c.Request().URL.Path, jwksPathSuffix) {
			return "GetJWKS"
		}

		return "Unknown"
	}

	return action
}

// ExtractResource extracts the user pool or user resource from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	// For JWKS endpoint, extract pool ID from the path.
	if strings.HasSuffix(c.Request().URL.Path, jwksPathSuffix) {
		trimmed := strings.TrimPrefix(c.Request().URL.Path, "/")
		poolID, _, _ := strings.Cut(trimmed, "/")

		return poolID
	}

	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var req struct {
		UserPoolID string `json:"UserPoolId,omitempty"`
		ClientID   string `json:"ClientId,omitempty"`
		Username   string `json:"Username,omitempty"`
	}

	_ = json.Unmarshal(body, &req)

	if req.UserPoolID != "" {
		return req.UserPoolID
	}

	if req.ClientID != "" {
		return req.ClientID
	}

	return req.Username
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		if strings.HasSuffix(c.Request().URL.Path, jwksPathSuffix) {
			return h.handleJWKS(c)
		}

		return service.HandleTarget(
			c, logger.Load(c.Request().Context()),
			"AWSCognitoIdentityProviderService", contentType,
			h.GetSupportedOperations(),
			h.dispatch,
			h.handleError,
		)
	}
}

func (h *Handler) dispatchTable() map[string]service.JSONOpFunc {
	table := map[string]service.JSONOpFunc{}
	maps.Copy(table, h.attributesOpsA())
	maps.Copy(table, h.authOpsA())
	maps.Copy(table, h.authTokensOpsA())
	maps.Copy(table, h.devicesOpsA())
	maps.Copy(table, h.groupsOpsA())
	maps.Copy(table, h.identityProvidersOpsA())
	maps.Copy(table, h.userPoolClientsOpsA())
	maps.Copy(table, h.userPoolsOpsA())
	maps.Copy(table, h.usersOpsA())
	maps.Copy(table, h.authEventsOps())
	maps.Copy(table, h.authTokensOpsB())
	maps.Copy(table, h.brandingOpsA())
	maps.Copy(table, h.devicesOpsB())
	maps.Copy(table, h.domainsOpsA())
	maps.Copy(table, h.identityProvidersOpsB())
	maps.Copy(table, h.mfaOpsA())
	maps.Copy(table, h.securityConfigOpsA())
	maps.Copy(table, h.tagsOps())
	maps.Copy(table, h.termsOps())
	maps.Copy(table, h.userImportOps())
	maps.Copy(table, h.userPoolClientsOpsB())
	maps.Copy(table, h.usersOpsB())
	maps.Copy(table, h.webauthnOps())
	maps.Copy(table, h.authOpsC())
	maps.Copy(table, h.mfaOpsB())
	maps.Copy(table, h.resourceServersOpsB())
	maps.Copy(table, h.userPoolClientsOpsC())
	maps.Copy(table, h.userPoolsOpsB())
	maps.Copy(table, h.usersOpsC())
	maps.Copy(table, h.attributesOpsC())
	maps.Copy(table, h.brandingOpsB())
	maps.Copy(table, h.domainsOpsB())
	maps.Copy(table, h.groupsOpsB())
	maps.Copy(table, h.identityProvidersOpsC())
	maps.Copy(table, h.securityConfigOpsB())
	maps.Copy(table, h.userPoolsOpsC())
	maps.Copy(table, h.usersOpsD())
	maps.Copy(table, h.usersOpsE())
	maps.Copy(table, h.userPoolReplicasOps())
	maps.Copy(table, h.provisionedLimitsOps())

	return table
}

func (h *Handler) dispatch(ctx context.Context, action string, body []byte) ([]byte, error) {
	fn, ok := h.ops[action]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUnknownAction, action)
	}

	result, err := fn(ctx, body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

func (h *Handler) handleError(_ context.Context, c *echo.Context, _ string, err error) error {
	errType, statusCode := resolveErrorType(err)

	return c.JSON(statusCode, service.JSONErrorResponse{
		Type:    errType,
		Message: err.Error(),
	})
}

// cognitoSentinelErrors maps sentinel errors to their Cognito exception type names.
// All Cognito errors return 400 Bad Request.
var cognitoSentinelErrors = []struct { //nolint:gochecknoglobals // package-level lookup table
	sentinel error
	typeName string
}{
	{ErrUserNotFound, ErrUserNotFound.Error()},
	{ErrUserPoolNotFound, ErrUserPoolNotFound.Error()},
	{ErrClientNotFound, ErrClientNotFound.Error()},
	{ErrExpiredCode, ErrExpiredCode.Error()},
	{ErrUsernameExists, ErrUsernameExists.Error()},
	{ErrNotAuthorized, ErrNotAuthorized.Error()},
	{ErrTokenUnauthorized, ErrTokenUnauthorized.Error()},
	{ErrInvalidPassword, ErrInvalidPassword.Error()},
	{ErrUserNotConfirmed, ErrUserNotConfirmed.Error()},
	{ErrPasswordResetRequired, ErrPasswordResetRequired.Error()},
	{ErrCodeMismatch, ErrCodeMismatch.Error()},
	{ErrInvalidUserPoolConfig, ErrInvalidUserPoolConfig.Error()},
	{ErrGroupNotFound, ErrGroupNotFound.Error()},
	{ErrAlreadyExists, ErrAlreadyExists.Error()},
	{ErrDuplicateProvider, ErrDuplicateProvider.Error()},
	{ErrInvalidParameter, ErrInvalidParameter.Error()},
	{ErrInvalidToken, ErrInvalidToken.Error()},
	{ErrDeviceNotFound, ErrDeviceNotFound.Error()},
	{ErrWebAuthnCredentialNotFound, ErrWebAuthnCredentialNotFound.Error()},
	{ErrAuthEventNotFound, ErrAuthEventNotFound.Error()},
	{ErrUserLambdaValidation, ErrUserLambdaValidation.Error()},
	{ErrUnexpectedLambda, ErrUnexpectedLambda.Error()},
	{ErrReplicaNotFound, ErrReplicaNotFound.Error()},
	{ErrServiceQuotaExceeded, ErrServiceQuotaExceeded.Error()},
	{ErrTermsNotFound, ErrTermsNotFound.Error()},
	{ErrTermsExists, ErrTermsExists.Error()},
	{ErrSecretNotFound, ErrSecretNotFound.Error()},
	{ErrLimitExceeded, ErrLimitExceeded.Error()},
	{errUnknownAction, "UnknownOperationException"},
}

func resolveErrorType(err error) (string, int) {
	for _, entry := range cognitoSentinelErrors {
		if errors.Is(err, entry.sentinel) {
			return entry.typeName, http.StatusBadRequest
		}
	}

	if _, ok := errors.AsType[*json.SyntaxError](err); ok {
		return errInvalidParameterException, http.StatusBadRequest
	}

	if _, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return errInvalidParameterException, http.StatusBadRequest
	}

	return "InternalFailure", http.StatusInternalServerError
}

// cognitoMaxResultsCap is the AWS upper bound on MaxResults/Limit for the
// Cognito IDP list operations (ListUserPools, ListUserPoolClients, ListUsers, ListTerms).
const cognitoMaxResultsCap = 60

// validateCognitoMaxResults clamps and validates a MaxResults/Limit value.
// AWS rejects values < 1 or > 60 with InvalidParameterException. A zero value
// means "unset" and defaults to the cap.
func validateCognitoMaxResults(maxResults int) (int, error) {
	if maxResults == 0 {
		return cognitoMaxResultsCap, nil
	}

	if maxResults < 1 || maxResults > cognitoMaxResultsCap {
		return 0, fmt.Errorf(
			"%w: MaxResults must be between 1 and %d", ErrInvalidParameter, cognitoMaxResultsCap)
	}

	return maxResults, nil
}

// wrapAccuracy adapts a typed handler function to the generic dispatch signature.
func wrapAccuracy[I any, O any](fn func(context.Context, *I) (*O, error)) service.JSONOpFunc {
	return service.WrapOp(fn)
}
