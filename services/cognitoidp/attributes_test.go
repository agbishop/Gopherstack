package cognitoidp_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cognitoidp"
)

func TestVerifyUserAttribute_RealCodeValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wantErrType string
		wantStatus  int
		useRealCode bool
	}{
		{
			name:        "correct_code_succeeds",
			useRealCode: true,
			wantStatus:  http.StatusOK,
		},
		{
			name:        "wrong_code_fails",
			useRealCode: false,
			wantStatus:  http.StatusBadRequest,
			wantErrType: "CodeMismatchException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Use the backend directly so we can extract the verification code.
			b := newTestBackend()
			h := cognitoidp.NewHandler(b, "us-east-1")

			poolRec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{
				"PoolName": "verify-attr-pool-" + tt.name,
			})
			require.Equal(t, http.StatusOK, poolRec.Code)
			var poolResp struct {
				UserPool struct {
					ID string `json:"Id"`
				} `json:"UserPool"`
			}
			require.NoError(t, json.Unmarshal(poolRec.Body.Bytes(), &poolResp))
			poolID := poolResp.UserPool.ID

			clientRec := doCognitoRequest(t, h, "CreateUserPoolClient", map[string]any{
				"UserPoolId": poolID,
				"ClientName": "test-client",
			})
			require.Equal(t, http.StatusOK, clientRec.Code)
			var clientResp struct {
				UserPoolClient struct {
					ClientID string `json:"ClientId"`
				} `json:"UserPoolClient"`
			}
			require.NoError(t, json.Unmarshal(clientRec.Body.Bytes(), &clientResp))
			clientID := clientResp.UserPoolClient.ClientID

			// Sign up and confirm a user with email attribute.
			signUpRec := doCognitoRequest(t, h, "SignUp", map[string]any{
				"ClientId": clientID,
				"Username": "verifyuser",
				"Password": "Passw0rd!",
				"UserAttributes": []map[string]string{
					{"Name": "email", "Value": "user@example.com"},
				},
			})
			require.Equal(t, http.StatusOK, signUpRec.Code, signUpRec.Body.String())

			adminConfRec := doCognitoRequest(t, h, "AdminConfirmSignUp", map[string]any{
				"UserPoolId": poolID,
				"Username":   "verifyuser",
			})
			require.Equal(t, http.StatusOK, adminConfRec.Code)

			// Authenticate to get access token.
			authResp := initiateAuth(t, h, clientID, "verifyuser")
			authResult, ok := authResp["AuthenticationResult"].(map[string]any)
			require.True(t, ok, "missing AuthenticationResult")
			accessToken, ok := authResult["AccessToken"].(string)
			require.True(t, ok, "missing AccessToken")

			// Request attribute verification code — stored in backend.
			getCodeRec := doCognitoRequest(t, h, "GetUserAttributeVerificationCode", map[string]any{
				"AccessToken":   accessToken,
				"AttributeName": "email",
			})
			require.Equal(t, http.StatusOK, getCodeRec.Code, getCodeRec.Body.String())

			// Retrieve the actual code from the backend (not sent in HTTP response — simulates delivery).
			realCode := b.GetAttrVerificationCodeForTest(poolID, "verifyuser", "email")
			require.NotEmpty(t, realCode, "expected verification code stored in backend")

			code := "000000"
			if tt.useRealCode {
				code = realCode
			}

			rec := doCognitoRequest(t, h, "VerifyUserAttribute", map[string]any{
				"AccessToken":   accessToken,
				"AttributeName": "email",
				"Code":          code,
			})
			assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())

			if tt.wantErrType != "" {
				var errResp struct {
					Type string `json:"__type"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
				assert.Equal(t, tt.wantErrType, errResp.Type)
			}
		})
	}
}

func TestAttrVerification_GetAndVerify(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("attr-verify-pool")
	require.NoError(t, err)
	client, err := b.CreateUserPoolClient(pool.ID, "attr-client")
	require.NoError(t, err)

	user, err := b.SignUp(client.ClientID, "attr-user", "Pass1234!", map[string]string{
		"email": "attr@example.com",
	})
	require.NoError(t, err)
	require.NoError(t, b.ConfirmSignUp(client.ClientID, "attr-user", user.ConfirmCode))

	result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "attr-user", "Pass1234!")
	require.NoError(t, err)
	accessToken := result.Tokens.AccessToken

	// Generate code.
	code, dest, medium, err := b.GetUserAttributeVerificationCode(accessToken, "email")
	require.NoError(t, err)
	assert.NotEmpty(t, code)
	assert.NotEmpty(t, dest)
	assert.Equal(t, "EMAIL", medium)
	assert.Contains(t, dest, "***")

	// Verify with code.
	require.NoError(t, b.VerifyUserAttributeWithCode(accessToken, "email", code))

	// User attribute should be verified.
	u, err := b.GetUser(accessToken)
	require.NoError(t, err)
	assert.Equal(t, "true", u.Attributes["email_verified"])
}

func TestAttrVerification_WrongCode(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("attr-verify-wrong-pool")
	require.NoError(t, err)
	client, err := b.CreateUserPoolClient(pool.ID, "attr-client2")
	require.NoError(t, err)

	user, err := b.SignUp(client.ClientID, "attr-user2", "Pass1234!", map[string]string{
		"email": "wrong@example.com",
	})
	require.NoError(t, err)
	require.NoError(t, b.ConfirmSignUp(client.ClientID, "attr-user2", user.ConfirmCode))

	result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "attr-user2", "Pass1234!")
	require.NoError(t, err)

	_, _, _, err = b.GetUserAttributeVerificationCode(result.Tokens.AccessToken, "email")
	require.NoError(t, err)

	// Wrong code.
	err = b.VerifyUserAttributeWithCode(result.Tokens.AccessToken, "email", "WRONG!")
	require.Error(t, err)
}

func TestAttrVerification_NoCodeGenerated(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("attr-nocode-pool")
	require.NoError(t, err)
	client, err := b.CreateUserPoolClient(pool.ID, "attr-nocode-client")
	require.NoError(t, err)

	user, err := b.SignUp(client.ClientID, "attr-nocode-user", "Pass1234!", map[string]string{
		"email": "nocode@example.com",
	})
	require.NoError(t, err)
	require.NoError(t, b.ConfirmSignUp(client.ClientID, "attr-nocode-user", user.ConfirmCode))

	result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "attr-nocode-user", "Pass1234!")
	require.NoError(t, err)

	// No code generated — verify should fail.
	err = b.VerifyUserAttributeWithCode(result.Tokens.AccessToken, "email", "123456")
	require.Error(t, err)
}

func TestAttrVerification_PhoneNumber(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("attr-phone-pool")
	require.NoError(t, err)
	client, err := b.CreateUserPoolClient(pool.ID, "phone-client")
	require.NoError(t, err)

	user, err := b.SignUp(client.ClientID, "phone-user", "Pass1234!", map[string]string{
		"phone_number": "+14155551234",
		"email":        "phone@example.com",
	})
	require.NoError(t, err)
	require.NoError(t, b.ConfirmSignUp(client.ClientID, "phone-user", user.ConfirmCode))

	result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "phone-user", "Pass1234!")
	require.NoError(t, err)

	code, dest, medium, err := b.GetUserAttributeVerificationCode(result.Tokens.AccessToken, "phone_number")
	require.NoError(t, err)
	assert.NotEmpty(t, code)
	assert.Equal(t, "SMS", medium)
	assert.Contains(t, dest, "+*******")
	assert.Contains(t, dest, "1234")
}

func TestAttrVerification_InvalidAttribute(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("attr-invalid-pool")
	require.NoError(t, err)
	client, err := b.CreateUserPoolClient(pool.ID, "attr-inv-client")
	require.NoError(t, err)

	user, err := b.SignUp(client.ClientID, "attr-inv-user", "Pass1234!", map[string]string{
		"email": "inv@example.com",
	})
	require.NoError(t, err)
	require.NoError(t, b.ConfirmSignUp(client.ClientID, "attr-inv-user", user.ConfirmCode))

	result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "attr-inv-user", "Pass1234!")
	require.NoError(t, err)

	// "name" is not verifiable.
	_, _, _, err = b.GetUserAttributeVerificationCode(result.Tokens.AccessToken, "name")
	require.Error(t, err)
}

func TestAttrVerification_Handler_GetAndVerify(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	poolID, clientID := setupHandlerPoolAndClient(t, h, "attr-handler-pool")

	rec := doCognitoRequest(t, h, "SignUp", map[string]any{
		"ClientId": clientID,
		"Username": "attr-handler-user",
		"Password": "Pass1234!",
		"UserAttributes": []map[string]string{
			{"Name": "email", "Value": "handler@example.com"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var signUpResp struct {
		CodeDeliveryDetails map[string]string `json:"CodeDeliveryDetails,omitempty"`
		UserConfirmed       bool              `json:"UserConfirmed,omitempty"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &signUpResp))

	if !signUpResp.UserConfirmed {
		confirmRec := doCognitoRequest(t, h, "ConfirmSignUp", map[string]any{
			"ClientId":         clientID,
			"Username":         "attr-handler-user",
			"ConfirmationCode": signUpResp.CodeDeliveryDetails["ConfirmationCode"],
		})
		require.Equal(t, http.StatusOK, confirmRec.Code)
	}

	accessToken := loginViaHandler(t, h, clientID, "attr-handler-user")

	// GetUserAttributeVerificationCode.
	rec = doCognitoRequest(t, h, "GetUserAttributeVerificationCode", map[string]any{
		"AccessToken":   accessToken,
		"AttributeName": "email",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var codeOut struct {
		CodeDeliveryDetails map[string]string `json:"CodeDeliveryDetails,omitempty"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &codeOut))
	assert.Equal(t, "EMAIL", codeOut.CodeDeliveryDetails["DeliveryMedium"])
	assert.NotEmpty(t, codeOut.CodeDeliveryDetails["Destination"])

	// Get code from backend for test.
	code := h.Backend.GetAttrVerificationCodeForTest(poolID, "attr-handler-user", "email")
	require.NotEmpty(t, code)

	// VerifyUserAttribute with real code.
	rec = doCognitoRequest(t, h, "VerifyUserAttribute", map[string]any{
		"AccessToken":   accessToken,
		"AttributeName": "email",
		"Code":          code,
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestEvictExpiredAttrVerificationCodes(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("evict-attr-pool")
	require.NoError(t, err)
	client, err := b.CreateUserPoolClient(pool.ID, "evict-client")
	require.NoError(t, err)

	user, err := b.SignUp(client.ClientID, "evict-user", "Pass1234!", map[string]string{
		"email": "evict@example.com",
	})
	require.NoError(t, err)
	require.NoError(t, b.ConfirmSignUp(client.ClientID, "evict-user", user.ConfirmCode))

	result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "evict-user", "Pass1234!")
	require.NoError(t, err)

	code, _, _, err := b.GetUserAttributeVerificationCode(result.Tokens.AccessToken, "email")
	require.NoError(t, err)
	assert.NotEmpty(t, code)

	// Evict — code should still be there (not expired).
	assert.Zero(t, b.EvictExpiredAttrVerificationCodes())

	// Code should still work since not expired.
	storedCode := b.GetAttrVerificationCodeForTest(pool.ID, "evict-user", "email")
	assert.NotEmpty(t, storedCode)
}

func TestGetUserAttributeVerifCode_MasksEmail(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("mask-email-pool")
	require.NoError(t, err)
	client, err := b.CreateUserPoolClient(pool.ID, "mask-client")
	require.NoError(t, err)

	user, err := b.SignUp(client.ClientID, "mask-user", "Pass1234!", map[string]string{
		"email": "johndoe@example.com",
	})
	require.NoError(t, err)
	require.NoError(t, b.ConfirmSignUp(client.ClientID, "mask-user", user.ConfirmCode))

	result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "mask-user", "Pass1234!")
	require.NoError(t, err)

	_, dest, medium, err := b.GetUserAttributeVerificationCode(result.Tokens.AccessToken, "email")
	require.NoError(t, err)
	assert.Equal(t, "EMAIL", medium)
	// Should start with "jo***" and contain the domain.
	assert.Contains(t, dest, "jo***")
	assert.Contains(t, dest, "@example.com")
}

// TestHandler_DeleteUserAttributes covers the HTTP handler for DeleteUserAttributes.
func TestHandler_DeleteUserAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantCode int
	}{
		{name: "success", wantCode: http.StatusOK},
		{name: "bad_token", wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, clientID := setupHandlerPoolAndClient(t, h, "del-attr-pool")

			var accessToken string

			if tt.name == "success" {
				signUpAndConfirmViaHandler(t, h, clientID, "del-attr-user")
				accessToken = loginViaHandler(t, h, clientID, "del-attr-user")
			} else {
				accessToken = "bad-token"
			}

			rec := doCognitoRequest(t, h, "DeleteUserAttributes", map[string]any{
				"AccessToken":        accessToken,
				"UserAttributeNames": []string{"email"},
			})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestHandler_VerifyUserAttribute covers the HTTP handler for VerifyUserAttribute.
func TestHandler_VerifyUserAttribute(t *testing.T) {
	t.Parallel()

	t.Run("bad_token", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := doCognitoRequest(t, h, "VerifyUserAttribute", map[string]any{
			"AccessToken":   "bad-token",
			"AttributeName": "email",
			"Code":          "123456",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("success_with_real_code", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		poolID, clientID := setupHandlerPoolAndClient(t, h, "verify-attr-pool2")

		// Sign up with email so GetUserAttributeVerificationCode works.
		rec := doCognitoRequest(t, h, "SignUp", map[string]any{
			"ClientId": clientID,
			"Username": "verify-attr-user2",
			"Password": "Pass1234!",
			"UserAttributes": []map[string]string{
				{"Name": "email", "Value": "verify@example.com"},
			},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var signUpResp struct {
			CodeDeliveryDetails map[string]string `json:"CodeDeliveryDetails,omitempty"`
			UserConfirmed       bool              `json:"UserConfirmed,omitempty"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &signUpResp))

		if !signUpResp.UserConfirmed {
			confirmRec := doCognitoRequest(t, h, "ConfirmSignUp", map[string]any{
				"ClientId":         clientID,
				"Username":         "verify-attr-user2",
				"ConfirmationCode": signUpResp.CodeDeliveryDetails["ConfirmationCode"],
			})
			require.Equal(t, http.StatusOK, confirmRec.Code)
		}

		accessToken := loginViaHandler(t, h, clientID, "verify-attr-user2")

		// Generate and store a verification code.
		_, _, _, err := h.Backend.GetUserAttributeVerificationCode(accessToken, "email")
		require.NoError(t, err)

		code := h.Backend.GetAttrVerificationCodeForTest(poolID, "verify-attr-user2", "email")
		require.NotEmpty(t, code)

		rec = doCognitoRequest(t, h, "VerifyUserAttribute", map[string]any{
			"AccessToken":   accessToken,
			"AttributeName": "email",
			"Code":          code,
		})
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// TestBackend_DeleteUserAttributes covers the backend DeleteUserAttributes function.
