package organizations_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/organizations"
)

// TestHandler_CreateAccount tests the HTTP handler for CreateAccount.
//
// The rejection cases assert AccountCount stays unchanged, not just the
// response status: validateCreateAccountInput used to write its rejection
// via h.writeError and return that call's (always-nil) result, so
// handleCreateAccount's `if err != nil` never fired and the account was
// created anyway. A status-only assertion still shows 400 here, because
// httptest.ResponseRecorder keeps the first WriteHeader call and ignores the
// handler's later successful write on top of it -- that is how this bug hid
// behind these exact cases before (gopherstack-3t96, the gopherstack-8haq
// shape).
func TestHandler_CreateAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "creates_account",
			body: map[string]any{
				"AccountName": "test-account",
				"Email":       "test@example.com",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing_name_fails",
			body:       map[string]any{"Email": "test@example.com"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_email_fails",
			body:       map[string]any{"AccountName": "test-account"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "invalid_iam_user_access_to_billing_fails",
			body: map[string]any{
				"AccountName":            "test-account",
				"Email":                  "test@example.com",
				"IamUserAccessToBilling": "MAYBE",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)
			h := organizations.NewHandler(b)

			before := organizations.AccountCount(b)

			rec := doRequest(t, h, "CreateAccount", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			after := organizations.AccountCount(b)
			if tt.wantStatus == http.StatusOK {
				assert.Equal(t, before+1, after, "a successful CreateAccount must add exactly one account")
			} else {
				assert.Equal(t, before, after, "a rejected CreateAccount must not create an account")
			}
		})
	}
}

func TestCreateAccountStatus(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateOrganization", map[string]any{"featureSet": "ALL"})

	// CreateAccount creates an async account
	rec := doRequest(t, h, "CreateAccount", map[string]any{
		"accountName": "test-account",
		"email":       "test@example.com",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&createResp))
	status := createResp["CreateAccountStatus"].(map[string]any)
	requestID, ok := status["Id"].(string)
	require.True(t, ok, "CreateAccountStatus.Id must be present")

	// ListCreateAccountStatus
	rec = doRequest(t, h, "ListCreateAccountStatus", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&listResp))
	statuses, ok := listResp["CreateAccountStatuses"].([]any)
	require.True(t, ok)

	found := false

	for _, s := range statuses {
		entry, entryOK := s.(map[string]any)
		if entryOK && entry["Id"] == requestID {
			found = true

			break
		}
	}

	assert.True(t, found, "ListCreateAccountStatus must include the just-created account's status")
}

// TestHandler_AccountErrors tests account handler error paths.
func TestHandler_AccountErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		op         string
		wantStatus int
	}{
		{
			name:       "describe_account_not_found",
			op:         "DescribeAccount",
			body:       map[string]any{"AccountId": "999999999999"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "move_account_not_found",
			op:   "MoveAccount",
			body: map[string]any{
				"AccountId":           "999999999999",
				"SourceParentId":      "r-root",
				"DestinationParentId": "r-root",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "describe_create_status_not_found",
			op:         "DescribeCreateAccountStatus",
			body:       map[string]any{"CreateAccountRequestId": "car-notexist"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "remove_account_not_found",
			op:         "RemoveAccountFromOrganization",
			body:       map[string]any{"AccountId": "999999999999"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newHandlerWithOrg(t)

			rec := doRequest(t, h, tt.op, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestCreateAccount_RoleName verifies RoleName defaults to OrganizationAccountAccessRole.
func TestCreateAccount_RoleName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		roleName     string
		wantRoleName string
	}{
		{name: "default_role", roleName: "", wantRoleName: "OrganizationAccountAccessRole"},
		{name: "custom_role", roleName: "MyCustomRole", wantRoleName: "MyCustomRole"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// Create org first.
			rec := doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})
			require.Equal(t, http.StatusOK, rec.Code)

			// Create account.
			body := map[string]any{
				"AccountName": "test-account",
				"Email":       "test@example.com",
			}
			if tt.roleName != "" {
				body["RoleName"] = tt.roleName
			}

			rec = doRequest(t, h, "CreateAccount", body)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

			// Describe account.
			status := resp["CreateAccountStatus"].(map[string]any)
			accountID := status["AccountId"].(string)

			rec = doRequest(t, h, "DescribeAccount", map[string]any{"AccountId": accountID})
			require.Equal(t, http.StatusOK, rec.Code)

			var descResp map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&descResp))

			acct := descResp["Account"].(map[string]any)
			roleName, _ := acct["RoleName"].(string)
			assert.Equal(t, tt.wantRoleName, roleName)
		})
	}
}

// TestCreateAccount_IamUserAccessToBilling verifies IamUserAccessToBilling
// validation. The invalid cases assert AccountCount stays unchanged, not
// just the response status -- see TestHandler_CreateAccount's doc comment
// for why a status-only assertion passes even when the account is created
// anyway (gopherstack-3t96, the gopherstack-8haq shape). This is the
// pre-existing test that specifically covers the buggy branch (an invalid
// IamUserAccessToBilling); strengthening it, not just adding a new one, is
// the point -- it previously masked the bug it was meant to catch.
func TestCreateAccount_IamUserAccessToBilling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		iamAccess     string
		wantIamAccess string
		wantStatus    int
	}{
		{name: "default_allow", iamAccess: "", wantStatus: http.StatusOK, wantIamAccess: "ALLOW"},
		{name: "explicit_allow", iamAccess: "ALLOW", wantStatus: http.StatusOK, wantIamAccess: "ALLOW"},
		{name: "explicit_deny", iamAccess: "DENY", wantStatus: http.StatusOK, wantIamAccess: "DENY"},
		{name: "invalid_value", iamAccess: "MAYBE", wantStatus: http.StatusBadRequest},
		{name: "invalid_lowercase", iamAccess: "allow", wantStatus: http.StatusBadRequest},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)
			h := organizations.NewHandler(b)

			before := organizations.AccountCount(b)

			body := map[string]any{
				"AccountName": "test-account",
				"Email":       fmt.Sprintf("test%d@example.com", i),
			}
			if tt.iamAccess != "" {
				body["IamUserAccessToBilling"] = tt.iamAccess
			}

			rec := doRequest(t, h, "CreateAccount", body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			after := organizations.AccountCount(b)
			if tt.wantStatus == http.StatusOK {
				assert.Equal(t, before+1, after, "a successful CreateAccount must add exactly one account")
			} else {
				assert.Equal(t, before, after, "a rejected CreateAccount must not create an account")
			}

			if tt.wantStatus == http.StatusOK && tt.wantIamAccess != "" {
				var resp map[string]any
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

				status := resp["CreateAccountStatus"].(map[string]any)
				accountID := status["AccountId"].(string)

				rec = doRequest(t, h, "DescribeAccount", map[string]any{"AccountId": accountID})
				require.Equal(t, http.StatusOK, rec.Code)

				var descResp map[string]any
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&descResp))

				acct := descResp["Account"].(map[string]any)
				iamAccess, _ := acct["IamUserAccessToBilling"].(string)
				assert.Equal(t, tt.wantIamAccess, iamAccess)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Item 5: CloseAccount sets PENDING_CLOSURE and rejects double-close
// ---------------------------------------------------------------------------

// TestCloseAccount_ViaHandler tests CloseAccount via HTTP handler.
func TestCloseAccount_ViaHandler(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "CreateAccount", map[string]any{
		"AccountName": "close-test",
		"Email":       "close@example.com",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&createResp))
	statusObj := createResp["CreateAccountStatus"].(map[string]any)
	accountID := statusObj["AccountId"].(string)

	// First close succeeds.
	rec = doRequest(t, h, "CloseAccount", map[string]any{"AccountId": accountID})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Second close should fail.
	rec = doRequest(t, h, "CloseAccount", map[string]any{"AccountId": accountID})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---------------------------------------------------------------------------
// Item 6: OU depth limit
// ---------------------------------------------------------------------------

// TestEmailUniqueness_ViaHandler tests email uniqueness through the handler.
func TestEmailUniqueness_ViaHandler(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})
	require.Equal(t, http.StatusOK, rec.Code)

	// First account with email.
	rec = doRequest(t, h, "CreateAccount", map[string]any{
		"AccountName": "AccountA",
		"Email":       "shared@example.com",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Second account with same email should fail.
	rec = doRequest(t, h, "CreateAccount", map[string]any{
		"AccountName": "AccountB",
		"Email":       "shared@example.com",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestCreateAccount_NoGovCloudID_ViaHandler verifies that the HTTP response
// for CreateAccount does not include the GovCloudAccountId key.
func TestCreateAccount_NoGovCloudID_ViaHandler(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})

	rec := doRequest(t, h, "CreateAccount", map[string]any{
		"AccountName": "commercial-only",
		"Email":       "commercial-only@example.com",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	statusObj, ok := resp["CreateAccountStatus"].(map[string]any)
	require.True(t, ok, "response must have CreateAccountStatus")

	_, hasGovCloud := statusObj["GovCloudAccountId"]
	assert.False(t, hasGovCloud,
		"CreateAccount response must not include GovCloudAccountId; got %v", statusObj)
}

// TestGovCloudAccount_HasGovCloudID verifies that CreateGovCloudAccount
// response DOES include GovCloudAccountId.
func TestGovCloudAccount_HasGovCloudID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})

	rec := doRequest(t, h, "CreateGovCloudAccount", map[string]any{
		"AccountName": "gov-only",
		"Email":       "gov-only@example.com",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	statusObj, ok := resp["CreateAccountStatus"].(map[string]any)
	require.True(t, ok, "response must have CreateAccountStatus")

	govID, hasGovCloud := statusObj["GovCloudAccountId"]
	assert.True(t, hasGovCloud, "CreateGovCloudAccount response must include GovCloudAccountId")
	assert.NotEmpty(t, govID, "GovCloudAccountId must not be empty")
}

// ---------------------------------------------------------------------------
// Item 27: TagResource / UntagResource / ListTagsForResource — TargetNotFoundException
// for non-existent resources (not InvalidInputException)
// ---------------------------------------------------------------------------

// TestHandler_ListAccounts tests the HTTP handler for ListAccounts.
func TestHandler_ListAccounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{
			name:       "lists_accounts",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newHandlerWithOrg(t)

			createAccountViaHandler(t, h, "test-account", "test@example.com")

			rec := doRequest(t, h, "ListAccounts", map[string]any{})
			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			accounts, ok := resp["Accounts"].([]any)
			require.True(t, ok)
			assert.NotEmpty(t, accounts)
		})
	}
}

// TestHandler_DescribeAccount tests the HTTP handler for DescribeAccount.
func TestHandler_DescribeAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
		notFound   bool
	}{
		{
			name:       "describes_account",
			wantStatus: http.StatusOK,
		},
		{
			name:       "not_found",
			wantStatus: http.StatusBadRequest,
			notFound:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newHandlerWithOrg(t)

			accountID := "000000000099"
			if !tt.notFound {
				accountID = createAccountViaHandler(t, h, "test-account", "test@example.com")
			}

			rec := doRequest(t, h, "DescribeAccount", map[string]any{"AccountId": accountID})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_DescribeCreateAccountStatus tests the HTTP handler for DescribeCreateAccountStatus.
func TestHandler_DescribeCreateAccountStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{
			name:       "describes_status",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newHandlerWithOrg(t)

			// Create account and capture status.
			rec := doRequest(t, h, "CreateAccount", map[string]any{
				"AccountName": "test-account",
				"Email":       "test@example.com",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var createResp map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&createResp))
			status := createResp["CreateAccountStatus"].(map[string]any)
			statusID := status["Id"].(string)

			rec = doRequest(t, h, "DescribeCreateAccountStatus", map[string]any{
				"CreateAccountRequestId": statusID,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_RemoveAccountFromOrganization tests the HTTP handler.
func TestHandler_RemoveAccountFromOrganization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{
			name:       "removes_account",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newHandlerWithOrg(t)
			accountID := createAccountViaHandler(t, h, "test-account", "test@example.com")

			rec := doRequest(t, h, "RemoveAccountFromOrganization", map[string]any{"AccountId": accountID})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_MoveAccount tests the HTTP handler for MoveAccount.
func TestHandler_MoveAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{
			name:       "moves_account",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, rootID := newHandlerWithOrg(t)
			accountID := createAccountViaHandler(t, h, "test-account", "test@example.com")

			// Create target OU.
			ouRec := doRequest(t, h, "CreateOrganizationalUnit", map[string]any{
				"ParentId": rootID,
				"Name":     "target-ou",
			})
			require.Equal(t, http.StatusOK, ouRec.Code)

			var ouResp map[string]any
			require.NoError(t, json.NewDecoder(ouRec.Body).Decode(&ouResp))
			ou := ouResp["OrganizationalUnit"].(map[string]any)
			ouID := ou["Id"].(string)

			rec := doRequest(t, h, "MoveAccount", map[string]any{
				"AccountId":           accountID,
				"SourceParentId":      rootID,
				"DestinationParentId": ouID,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_CloseAccount tests the CloseAccount operation.
func TestHandler_CloseAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body          map[string]any
		name          string
		wantStatus    int
		createAccount bool
	}{
		{
			name:          "closes_existing_account",
			createAccount: true,
			wantStatus:    http.StatusOK,
		},
		{
			name:       "missing_account_id",
			body:       map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "account_not_found",
			body:       map[string]any{"AccountId": "999999999999"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateOrganization", map[string]any{"FeatureSet": "ALL"})

			body := tt.body

			if tt.createAccount {
				// Create an account first then close it.
				acctRec := doRequest(t, h, "CreateAccount", map[string]any{
					"AccountName": "close-test",
					"Email":       "close@example.com",
				})
				require.Equal(t, http.StatusOK, acctRec.Code)

				var acctResp map[string]any
				require.NoError(t, json.NewDecoder(acctRec.Body).Decode(&acctResp))

				status := acctResp["CreateAccountStatus"].(map[string]any)
				body = map[string]any{"AccountId": status["AccountId"]}
			}

			rec := doRequest(t, h, "CloseAccount", body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_CreateGovCloudAccount tests the CreateGovCloudAccount
// operation. The rejection cases assert AccountCount stays unchanged --
// handleCreateGovCloudAccount shares validateCreateAccountInput with
// handleCreateAccount, so it has the same gopherstack-3t96 shape and the
// same status-only-assertion blind spot; see TestHandler_CreateAccount's
// doc comment.
func TestHandler_CreateGovCloudAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
		wantGovID  bool
	}{
		{
			name: "creates_govcloud_account",
			body: map[string]any{
				"AccountName": "govcloud-test",
				"Email":       "gov@example.com",
			},
			wantStatus: http.StatusOK,
			wantGovID:  true,
		},
		{
			name:       "missing_account_name",
			body:       map[string]any{"Email": "gov@example.com"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_email",
			body:       map[string]any{"AccountName": "gov-test"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "invalid_iam_user_access_to_billing_fails",
			body: map[string]any{
				"AccountName":            "gov-test",
				"Email":                  "gov@example.com",
				"IamUserAccessToBilling": "MAYBE",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)
			h := organizations.NewHandler(b)

			before := organizations.AccountCount(b)

			rec := doRequest(t, h, "CreateGovCloudAccount", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			after := organizations.AccountCount(b)
			if tt.wantStatus == http.StatusOK {
				assert.Equal(t, before+1, after, "a successful CreateGovCloudAccount must add exactly one account")
			} else {
				assert.Equal(t, before, after, "a rejected CreateGovCloudAccount must not create an account")
			}

			if tt.wantGovID {
				var resp map[string]any
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

				status, ok := resp["CreateAccountStatus"].(map[string]any)
				require.True(t, ok, "response must have CreateAccountStatus")
				assert.NotEmpty(t, status["AccountId"])
				assert.NotEmpty(t, status["GovCloudAccountId"])
				assert.Equal(t, "SUCCEEDED", status["State"])
			}
		})
	}
}
