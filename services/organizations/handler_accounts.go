package organizations

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

const iamAccessAllow = "ALLOW"

// Validation errors returned unwritten by validateCreateAccountInput; both
// callers map them to the same InvalidInputException/400 response, so no
// dispatch on which one fired is needed.
var (
	errAccountNameRequired = errors.New("AccountName is required")
	errEmailRequired       = errors.New("email is required")
	errInvalidIamAccess    = errors.New("IamUserAccessToBilling must be ALLOW or DENY")
)

type createAccountRequest struct {
	AccountName            string `json:"AccountName"`
	Email                  string `json:"Email"`
	IamUserAccessToBilling string `json:"IamUserAccessToBilling,omitempty"`
	RoleName               string `json:"RoleName,omitempty"`
	Tags                   []Tag  `json:"Tags,omitempty"`
}

type createAccountResponse struct {
	CreateAccountStatus CreateAccountStatus `json:"CreateAccountStatus"`
}

type describeCreateAccountStatusRequest struct {
	CreateAccountRequestID string `json:"CreateAccountRequestId"`
}

type describeCreateAccountStatusResponse struct {
	CreateAccountStatus CreateAccountStatus `json:"CreateAccountStatus"`
}

type describeAccountRequest struct {
	AccountID string `json:"AccountId"`
}

type accountObject struct {
	ID                     string   `json:"Id"`
	ARN                    string   `json:"Arn"`
	Name                   string   `json:"Name"`
	Email                  string   `json:"Email"`
	Status                 string   `json:"Status"`
	JoinedMethod           string   `json:"JoinedMethod"`
	RoleName               string   `json:"RoleName,omitempty"`
	IamUserAccessToBilling string   `json:"IamUserAccessToBilling,omitempty"`
	Paths                  []string `json:"Paths,omitempty"`
	JoinedAt               float64  `json:"JoinedTimestamp"`
}

type describeAccountResponse struct {
	Account accountObject `json:"Account"`
}

type listAccountsRequest struct {
	NextToken  string `json:"NextToken,omitempty"`
	MaxResults int    `json:"MaxResults,omitempty"`
}

type listAccountsResponse struct {
	NextToken string          `json:"NextToken,omitempty"`
	Accounts  []accountObject `json:"Accounts"`
}

type removeAccountFromOrganizationRequest struct {
	AccountID string `json:"AccountId"`
}

type moveAccountRequest struct {
	AccountID           string `json:"AccountId"`
	SourceParentID      string `json:"SourceParentId"`
	DestinationParentID string `json:"DestinationParentId"`
}

// -- CloseAccount --

type closeAccountRequest struct {
	AccountID string `json:"AccountId"`
}

// -- CreateGovCloudAccount --

type createGovCloudAccountRequest struct {
	AccountName            string `json:"AccountName"`
	Email                  string `json:"Email"`
	IamUserAccessToBilling string `json:"IamUserAccessToBilling,omitempty"`
	RoleName               string `json:"RoleName,omitempty"`
	Tags                   []Tag  `json:"Tags,omitempty"`
}

type createGovCloudAccountResponse struct {
	CreateAccountStatus CreateAccountStatus `json:"CreateAccountStatus"`
}

// -- ListCreateAccountStatus --

type listCreateAccountStatusRequest struct {
	NextToken  string   `json:"NextToken,omitempty"`
	States     []string `json:"States,omitempty"`
	MaxResults int      `json:"MaxResults,omitempty"`
}

type listCreateAccountStatusResponse struct {
	NextToken             string                `json:"NextToken,omitempty"`
	CreateAccountStatuses []CreateAccountStatus `json:"CreateAccountStatuses"`
}

// -- ListAccountsWithInvalidEffectivePolicy --

type listAccountsWithInvalidEffectivePolicyRequest struct {
	PolicyType string `json:"PolicyType"`
	NextToken  string `json:"NextToken,omitempty"`
}

type listAccountsWithInvalidEffectivePolicyResponse struct {
	NextToken string          `json:"NextToken,omitempty"`
	Accounts  []accountObject `json:"Accounts"`
}

// dispatchAccount handles account operations.
func (h *Handler) dispatchAccount(c *echo.Context, op string, body []byte) (bool, error) {
	switch op {
	case "ListAccounts":
		return true, h.handleListAccounts(c, body)
	case "CreateAccount":
		return true, h.handleCreateAccount(c, body)
	case "DescribeCreateAccountStatus":
		return true, h.handleDescribeCreateAccountStatus(c, body)
	case "DescribeAccount":
		return true, h.handleDescribeAccount(c, body)
	case "RemoveAccountFromOrganization":
		return true, h.handleRemoveAccountFromOrganization(c, body)
	case "MoveAccount":
		return true, h.handleMoveAccount(c, body)
	case "CloseAccount":
		return true, h.handleCloseAccount(c, body)
	case "CreateGovCloudAccount":
		return true, h.handleCreateGovCloudAccount(c, body)
	case "ListCreateAccountStatus":
		return true, h.handleListCreateAccountStatus(c, body)
	case "ListAccountsWithInvalidEffectivePolicy":
		return true, h.handleListAccountsWithInvalidEffectivePolicy(c, body)
	}

	return false, nil
}

// ----------------------------------------
// Account handlers
// ----------------------------------------

func (h *Handler) handleListAccounts(c *echo.Context, body []byte) error {
	var req listAccountsRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return h.writeError(c, http.StatusBadRequest, "SerializationException", "invalid request body")
		}
	}

	accounts, err := h.Backend.ListAccounts()
	if err != nil {
		return h.handleBackendError(c, err)
	}

	objs := make([]accountObject, 0, len(accounts))
	for _, a := range accounts {
		objs = append(objs, toAccountObject(a))
	}

	p := page.New(objs, req.NextToken, req.MaxResults, defaultMaxResults)

	return c.JSON(http.StatusOK, listAccountsResponse{Accounts: p.Data, NextToken: p.Next})
}

// validateCreateAccountInput validates and normalises the common fields
// shared by CreateAccount and CreateGovCloudAccount requests, returning a
// raw unwritten error so both callers can map and write it exactly once.
// It used to write its own rejection via h.writeError and return that
// call's (always-nil) result, so the callers' `if err != nil` never fired
// and the account got created anyway (gopherstack-3t96, the
// gopherstack-8haq shape).
func (h *Handler) validateCreateAccountInput(
	accountName, email, roleName, iamAccess string,
) (string, string, error) {
	if accountName == "" {
		return "", "", errAccountNameRequired
	}

	if email == "" {
		return "", "", errEmailRequired
	}

	// Default RoleName.
	if roleName == "" {
		roleName = "OrganizationAccountAccessRole"
	}

	// Validate IamUserAccessToBilling.
	if iamAccess == "" {
		iamAccess = iamAccessAllow
	}
	if iamAccess != iamAccessAllow && iamAccess != "DENY" {
		return "", "", errInvalidIamAccess
	}

	return roleName, iamAccess, nil
}

func (h *Handler) handleCreateAccount(c *echo.Context, body []byte) error {
	var req createAccountRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return h.writeError(c, http.StatusBadRequest, "SerializationException", "invalid request body")
	}

	roleName, iamAccess, err := h.validateCreateAccountInput(
		req.AccountName, req.Email, req.RoleName, req.IamUserAccessToBilling,
	)
	if err != nil {
		return h.writeError(c, http.StatusBadRequest, "InvalidInputException", err.Error())
	}

	status, err := h.Backend.CreateAccount(req.AccountName, req.Email, roleName, iamAccess, req.Tags)
	if err != nil {
		return h.handleBackendError(c, err)
	}

	return c.JSON(http.StatusOK, createAccountResponse{CreateAccountStatus: *status})
}

func (h *Handler) handleDescribeCreateAccountStatus(c *echo.Context, body []byte) error {
	var req describeCreateAccountStatusRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return h.writeError(c, http.StatusBadRequest, "SerializationException", "invalid request body")
	}

	status, err := h.Backend.DescribeCreateAccountStatus(req.CreateAccountRequestID)
	if err != nil {
		return h.handleBackendError(c, err)
	}

	return c.JSON(http.StatusOK, describeCreateAccountStatusResponse{CreateAccountStatus: *status})
}

func (h *Handler) handleDescribeAccount(c *echo.Context, body []byte) error {
	var req describeAccountRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return h.writeError(c, http.StatusBadRequest, "SerializationException", "invalid request body")
	}

	acct, err := h.Backend.DescribeAccount(req.AccountID)
	if err != nil {
		return h.handleBackendError(c, err)
	}

	return c.JSON(http.StatusOK, describeAccountResponse{Account: toAccountObject(acct)})
}

func (h *Handler) handleRemoveAccountFromOrganization(c *echo.Context, body []byte) error {
	var req removeAccountFromOrganizationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return h.writeError(c, http.StatusBadRequest, "SerializationException", "invalid request body")
	}

	if err := h.Backend.RemoveAccountFromOrganization(req.AccountID); err != nil {
		return h.handleBackendError(c, err)
	}

	return c.JSON(http.StatusOK, struct{}{})
}

func (h *Handler) handleMoveAccount(c *echo.Context, body []byte) error {
	var req moveAccountRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return h.writeError(c, http.StatusBadRequest, "SerializationException", "invalid request body")
	}

	if err := h.Backend.MoveAccount(req.AccountID, req.SourceParentID, req.DestinationParentID); err != nil {
		return h.handleBackendError(c, err)
	}

	return c.JSON(http.StatusOK, struct{}{})
}

func (h *Handler) handleCloseAccount(c *echo.Context, body []byte) error {
	var req closeAccountRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return h.writeError(c, http.StatusBadRequest, "SerializationException", "invalid request body")
	}

	if req.AccountID == "" {
		return h.writeError(c, http.StatusBadRequest, "InvalidInputException", "AccountId is required")
	}

	if err := h.Backend.CloseAccount(req.AccountID); err != nil {
		return h.handleBackendError(c, err)
	}

	return c.JSON(http.StatusOK, struct{}{})
}

func (h *Handler) handleCreateGovCloudAccount(c *echo.Context, body []byte) error {
	var req createGovCloudAccountRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return h.writeError(c, http.StatusBadRequest, "SerializationException", "invalid request body")
	}

	roleName, iamAccess, err := h.validateCreateAccountInput(
		req.AccountName, req.Email, req.RoleName, req.IamUserAccessToBilling,
	)
	if err != nil {
		return h.writeError(c, http.StatusBadRequest, "InvalidInputException", err.Error())
	}

	status, err := h.Backend.CreateGovCloudAccount(req.AccountName, req.Email, roleName, iamAccess, req.Tags)
	if err != nil {
		return h.handleBackendError(c, err)
	}

	return c.JSON(http.StatusOK, createGovCloudAccountResponse{CreateAccountStatus: *status})
}

func (h *Handler) handleListCreateAccountStatus(c *echo.Context, body []byte) error {
	var req listCreateAccountStatusRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return h.writeError(c, http.StatusBadRequest, "SerializationException", "invalid request body")
	}

	statuses, err := h.Backend.ListCreateAccountStatus(req.States)
	if err != nil {
		return h.handleBackendError(c, err)
	}

	objs := make([]CreateAccountStatus, 0, len(statuses))
	for _, s := range statuses {
		objs = append(objs, *s)
	}

	p := page.New(objs, req.NextToken, req.MaxResults, defaultMaxResults)

	return c.JSON(http.StatusOK, listCreateAccountStatusResponse{CreateAccountStatuses: p.Data, NextToken: p.Next})
}

func (h *Handler) handleListAccountsWithInvalidEffectivePolicy(c *echo.Context, body []byte) error {
	var req listAccountsWithInvalidEffectivePolicyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return h.writeError(c, http.StatusBadRequest, "SerializationException", "invalid request body")
	}

	if req.PolicyType == "" {
		return h.writeError(c, http.StatusBadRequest, "InvalidInputException", "PolicyType is required")
	}

	accounts, err := h.Backend.ListAccountsWithInvalidEffectivePolicy(req.PolicyType)
	if err != nil {
		return h.handleBackendError(c, err)
	}

	objs := make([]accountObject, 0, len(accounts))
	for _, a := range accounts {
		objs = append(objs, toAccountObject(a))
	}

	return c.JSON(http.StatusOK, listAccountsWithInvalidEffectivePolicyResponse{Accounts: objs})
}

func toAccountObject(a *Account) accountObject {
	return accountObject{
		ID:                     a.ID,
		ARN:                    a.ARN,
		Name:                   a.Name,
		Email:                  a.Email,
		Status:                 a.Status,
		JoinedMethod:           a.JoinedMethod,
		JoinedAt:               epochSeconds(a.JoinedAt),
		RoleName:               a.RoleName,
		IamUserAccessToBilling: a.IamUserAccessToBilling,
		Paths:                  a.Paths,
	}
}
