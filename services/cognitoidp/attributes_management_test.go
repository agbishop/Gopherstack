package cognitoidp_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cognitoidp"
)

func TestBackend_DeleteUserAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{name: "success"},
		{name: "bad_token", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if tt.name == "bad_token" {
				err := b.DeleteUserAttributes("bad-token", []string{"email"})
				require.Error(t, err)

				return
			}

			pool, err := b.CreateUserPool("del-attr-pool")
			require.NoError(t, err)

			client, err := b.CreateUserPoolClient(pool.ID, "dac")
			require.NoError(t, err)

			user, err := b.SignUp(
				client.ClientID,
				"del-attr-user",
				"Pass1234!",
				map[string]string{"email": "x@example.com"},
			)
			require.NoError(t, err)
			require.NoError(t, b.ConfirmSignUp(client.ClientID, "del-attr-user", user.ConfirmCode))

			result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "del-attr-user", "Pass1234!")
			require.NoError(t, err)
			require.NotNil(t, result.Tokens)

			err = b.DeleteUserAttributes(result.Tokens.AccessToken, []string{"email"})
			require.NoError(t, err)
		})
	}
}

// TestBackend_VerifyUserAttribute covers the backend VerifyUserAttribute function.
func TestBackend_VerifyUserAttribute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{name: "success"},
		{name: "bad_token", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if tt.name == "bad_token" {
				err := b.VerifyUserAttribute("bad-token", "email", "123456")
				require.Error(t, err)

				return
			}

			pool, err := b.CreateUserPool("verify-attr-pool")
			require.NoError(t, err)

			client, err := b.CreateUserPoolClient(pool.ID, "vac")
			require.NoError(t, err)

			user, err := b.SignUp(client.ClientID, "verify-attr-user", "Pass1234!", map[string]string{})
			require.NoError(t, err)
			require.NoError(t, b.ConfirmSignUp(client.ClientID, "verify-attr-user", user.ConfirmCode))

			result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "verify-attr-user", "Pass1234!")
			require.NoError(t, err)
			require.NotNil(t, result.Tokens)

			// VerifyUserAttribute is a no-op stub in the backend; just check it doesn't error.
			err = b.VerifyUserAttribute(result.Tokens.AccessToken, "email", "123456")
			require.NoError(t, err)
		})
	}
}

func TestHandler_UpdateUserAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantCode int
		badToken bool
	}{
		{
			name:     "success",
			wantCode: http.StatusOK,
		},
		{
			name:     "invalid_token",
			wantCode: http.StatusBadRequest,
			badToken: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			poolRec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{"PoolName": "attr-pool"})
			var poolResp map[string]map[string]any
			_ = json.Unmarshal(poolRec.Body.Bytes(), &poolResp)
			poolID := poolResp["UserPool"]["Id"].(string)

			clientRec := doCognitoRequest(t, h, "CreateUserPoolClient", map[string]any{
				"UserPoolId": poolID,
				"ClientName": "c",
			})
			var clientResp map[string]map[string]any
			_ = json.Unmarshal(clientRec.Body.Bytes(), &clientResp)
			clientID := clientResp["UserPoolClient"]["ClientId"].(string)

			doCognitoRequest(t, h, "AdminCreateUser", map[string]any{
				"UserPoolId":        poolID,
				"Username":          "attruser",
				"TemporaryPassword": "Temp123!",
			})
			doCognitoRequest(t, h, "AdminSetUserPassword", map[string]any{
				"UserPoolId": poolID,
				"Username":   "attruser",
				"Password":   "PermPass456!",
				"Permanent":  true,
			})

			authRec := doCognitoRequest(t, h, "AdminInitiateAuth", map[string]any{
				"UserPoolId": poolID,
				"ClientId":   clientID,
				"AuthFlow":   "USER_PASSWORD_AUTH",
				"AuthParameters": map[string]string{
					"USERNAME": "attruser",
					"PASSWORD": "PermPass456!",
				},
			})
			require.Equal(t, http.StatusOK, authRec.Code)

			var authData map[string]map[string]any
			require.NoError(t, json.Unmarshal(authRec.Body.Bytes(), &authData))
			accessToken := authData["AuthenticationResult"]["AccessToken"].(string)

			if tt.badToken {
				accessToken = "invalid-token"
			}

			rec := doCognitoRequest(t, h, "UpdateUserAttributes", map[string]any{
				"AccessToken": accessToken,
				"UserAttributes": []map[string]any{
					{"Name": "custom:role", "Value": "editor"},
				},
			})
			assert.Equal(t, tt.wantCode, rec.Code)

			if !tt.badToken {
				// Verify attribute was updated via GetUser.
				guRec := doCognitoRequest(t, h, "GetUser", map[string]any{"AccessToken": accessToken})
				assert.Equal(t, http.StatusOK, guRec.Code)
				assert.Contains(t, guRec.Body.String(), "editor")
			}
		})
	}
}

func TestHandler_AdminUpdateUserAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		username string
		wantCode int
	}{
		{
			name:     "success",
			username: "attruser",
			wantCode: http.StatusOK,
		},
		{
			name:     "user_not_found",
			username: "nobody",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			poolRec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{"PoolName": "admin-attr-pool"})
			var poolResp map[string]map[string]any
			_ = json.Unmarshal(poolRec.Body.Bytes(), &poolResp)
			poolID := poolResp["UserPool"]["Id"].(string)

			doCognitoRequest(t, h, "AdminCreateUser", map[string]any{
				"UserPoolId":        poolID,
				"Username":          "attruser",
				"TemporaryPassword": "Temp123!",
			})

			rec := doCognitoRequest(t, h, "AdminUpdateUserAttributes", map[string]any{
				"UserPoolId": poolID,
				"Username":   tt.username,
				"UserAttributes": []map[string]any{
					{"Name": "custom:role", "Value": "admin"},
				},
			})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestHandler_AddCustomAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(h *cognitoidp.Handler) string
		body     func(poolID string) map[string]any
		name     string
		wantCode int
	}{
		{
			name: "success",
			setup: func(h *cognitoidp.Handler) string {
				rec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{"PoolName": "test-pool"})
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				return resp["UserPool"].(map[string]any)["Id"].(string)
			},
			body: func(poolID string) map[string]any {
				return map[string]any{
					"UserPoolId": poolID,
					"CustomAttributes": []map[string]any{
						{
							"Name":              "custom:department",
							"AttributeDataType": "String",
							"Mutable":           true,
						},
					},
				}
			},
			wantCode: http.StatusOK,
		},
		{
			name:  "pool_not_found",
			setup: func(_ *cognitoidp.Handler) string { return "us-east-1_NOTEXIST" },
			body: func(poolID string) map[string]any {
				return map[string]any{
					"UserPoolId":       poolID,
					"CustomAttributes": []map[string]any{{"Name": "custom:x"}},
				}
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			poolID := tt.setup(h)
			rec := doCognitoRequest(t, h, "AddCustomAttributes", tt.body(poolID))
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestHandler_AdminDeleteUserAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupAttrs map[string]any
		name       string
		attrNames  []string
		wantCode   int
		setupUser  bool
	}{
		{
			name:      "success",
			setupUser: true,
			setupAttrs: map[string]any{
				"email": "test@example.com",
				"phone": "+1234567890",
			},
			attrNames: []string{"phone"},
			wantCode:  http.StatusOK,
		},
		{
			name:      "user_not_found",
			setupUser: false,
			attrNames: []string{"email"},
			wantCode:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			poolRec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{"PoolName": "attr-pool"})
			var poolResp map[string]any
			require.NoError(t, json.Unmarshal(poolRec.Body.Bytes(), &poolResp))
			poolID := poolResp["UserPool"].(map[string]any)["Id"].(string)

			username := "deleteattr-user"
			if tt.setupUser {
				userAttrs := make([]map[string]any, 0)
				for k, v := range tt.setupAttrs {
					userAttrs = append(userAttrs, map[string]any{"Name": k, "Value": v})
				}
				doCognitoRequest(t, h, "AdminCreateUser", map[string]any{
					"UserPoolId":        poolID,
					"Username":          username,
					"TemporaryPassword": "TempPass123!",
					"UserAttributes":    userAttrs,
				})
			}

			rec := doCognitoRequest(t, h, "AdminDeleteUserAttributes", map[string]any{
				"UserPoolId":         poolID,
				"Username":           username,
				"UserAttributeNames": tt.attrNames,
			})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestAddCustomAttributes_RequiresCustomPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		attrName string
		wantCode int
	}{
		{
			name:     "valid_custom_prefix",
			attrName: "custom:role",
			wantCode: http.StatusOK,
		},
		{
			name:     "missing_custom_prefix",
			attrName: "role",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "email_no_prefix_rejected",
			attrName: "email",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			poolRec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{"PoolName": "prefix-pool"})
			var poolResp map[string]any
			require.NoError(t, json.Unmarshal(poolRec.Body.Bytes(), &poolResp))
			poolID := poolResp["UserPool"].(map[string]any)["Id"].(string)

			rec := doCognitoRequest(t, h, "AddCustomAttributes", map[string]any{
				"UserPoolId": poolID,
				"CustomAttributes": []map[string]any{
					{"Name": tt.attrName, "AttributeDataType": "String"},
				},
			})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestSortedAttributes(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	poolRec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{"PoolName": "sorted-attrs-pool"})
	var poolResp map[string]any
	require.NoError(t, json.Unmarshal(poolRec.Body.Bytes(), &poolResp))
	poolID := poolResp["UserPool"].(map[string]any)["Id"].(string)

	doCognitoRequest(t, h, "AdminCreateUser", map[string]any{
		"UserPoolId":        poolID,
		"Username":          "attr-user",
		"TemporaryPassword": "TempPass123!",
		"UserAttributes": []map[string]any{
			{"Name": "zz_last", "Value": "z"},
			{"Name": "aa_first", "Value": "a"},
			{"Name": "mm_middle", "Value": "m"},
		},
	})

	rec := doCognitoRequest(t, h, "AdminGetUser", map[string]any{
		"UserPoolId": poolID,
		"Username":   "attr-user",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	attrs := resp["UserAttributes"].([]any)
	require.GreaterOrEqual(t, len(attrs), 3)

	names := make([]string, 0, len(attrs))
	for _, a := range attrs {
		names = append(names, a.(map[string]any)["Name"].(string))
	}

	// Verify sorted order.
	for i := 1; i < len(names); i++ {
		assert.LessOrEqual(t, names[i-1], names[i], "attributes should be sorted by name")
	}
}

// TestParityB_IDTokenUserAttributeClaims verifies that email and other user attrs appear in ID tokens.
func TestIDTokenUserAttributeClaims(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	poolRec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{
		"PoolName":               "attr-pool",
		"AutoVerifiedAttributes": []string{"email"},
	})
	require.Equal(t, http.StatusOK, poolRec.Code)

	var poolResp map[string]any
	require.NoError(t, json.Unmarshal(poolRec.Body.Bytes(), &poolResp))
	poolID := poolResp["UserPool"].(map[string]any)["Id"].(string)

	clientRec := doCognitoRequest(t, h, "CreateUserPoolClient", map[string]any{
		"UserPoolId": poolID,
		"ClientName": "attr-client",
		"ExplicitAuthFlows": []string{
			"ALLOW_USER_PASSWORD_AUTH",
			"ALLOW_REFRESH_TOKEN_AUTH",
		},
	})
	require.Equal(t, http.StatusOK, clientRec.Code)

	var clientResp map[string]any
	require.NoError(t, json.Unmarshal(clientRec.Body.Bytes(), &clientResp))
	clientID := clientResp["UserPoolClient"].(map[string]any)["ClientId"].(string)

	signupRec := doCognitoRequest(t, h, "SignUp", map[string]any{
		"ClientId": clientID,
		"Username": "attruser",
		"Password": "Pass1234!",
		"UserAttributes": []map[string]string{
			{"Name": "email", "Value": "attruser@example.com"},
		},
	})
	require.Equal(t, http.StatusOK, signupRec.Code)

	var signupResp map[string]any
	require.NoError(t, json.Unmarshal(signupRec.Body.Bytes(), &signupResp))

	// AutoVerifiedAttributes only selects which channel receives the confirmation
	// code; it does not skip confirmation itself (see TestSignUpWithValidation_AutoVerify).
	require.False(t, signupResp["UserConfirmed"].(bool), "user should not be auto-confirmed")

	confirmRec := doCognitoRequest(t, h, "AdminConfirmSignUp", map[string]any{
		"UserPoolId": poolID,
		"Username":   "attruser",
	})
	require.Equal(t, http.StatusOK, confirmRec.Code)

	authRec := doCognitoRequest(t, h, "InitiateAuth", map[string]any{
		"ClientId": clientID,
		"AuthFlow": "USER_PASSWORD_AUTH",
		"AuthParameters": map[string]string{
			"USERNAME": "attruser",
			"PASSWORD": "Pass1234!",
		},
	})
	require.Equal(t, http.StatusOK, authRec.Code)

	var authResp map[string]any
	require.NoError(t, json.Unmarshal(authRec.Body.Bytes(), &authResp))
	idToken := authResp["AuthenticationResult"].(map[string]any)["IdToken"].(string)

	claims := decodeJWTPayload(t, idToken)
	assert.Equal(t, "attruser@example.com", claims["email"], "email must appear in ID token")
	assert.Equal(t, "true", claims["email_verified"], "email_verified must appear in ID token")
}

// TestParityB_AdminCreateUserSubInAttributes verifies sub appears in AdminCreateUser response.
//
// gopherstack-zquj: this read user["UserAttributes"] until 2026-08-22, which
// happened to match the wrong key adminUserJSON was tagged with at the time
// -- a raw-body assertion of the exact key the author typed, ratifying that
// bug rather than catching it. The real UserType member (and now this
// backend's own wire tag) is "Attributes"; see
// TestAdminCreateUser_UserAttributesKey_RealSDKClient
// (wire_field_fixes_test.go) for the real-SDK-client-side proof.
func TestAdminCreateUserSubInAttributes(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	poolID, _ := setupPoolAndClientNamed(t, h, "sub-pool", "sub-client")

	rec := doCognitoRequest(t, h, "AdminCreateUser", map[string]any{
		"UserPoolId":        poolID,
		"Username":          "subuser",
		"TemporaryPassword": "Temp1234!",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	user := resp["User"].(map[string]any)
	attrs := user["Attributes"].([]any)

	var subAttr map[string]any

	for _, a := range attrs {
		attr := a.(map[string]any)
		if attr["Name"].(string) == "sub" {
			subAttr = attr

			break
		}
	}
	require.NotNil(t, subAttr, "sub attribute must be present in AdminCreateUser response")
	assert.NotEmpty(t, subAttr["Value"], "sub must have a value")
}
