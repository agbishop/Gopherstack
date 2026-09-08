package cognitoidp_test

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cognitoidp"
)

// TestCompleteness_StubOperations calls every completeness stub that is not
// overridden by the accuracy dispatch table and asserts HTTP 200.
func TestStubOperations(t *testing.T) {
	t.Parallel()

	// Completeness stubs that are NOT overridden by accuracy dispatch.
	// Each is a no-op returning an empty 200 OK response.
	// Ops still returning HTTP 200 with arbitrary/empty inputs (no pool validation required).
	// Ops with real stateful backends (requiring valid UserPoolId) are tested in completeness_impl_test.go.
	stubs := []string{
		"DescribeUserPoolDomain",
		"GetCSVHeader",
		"ListTagsForResource",
		"TagResource",
		"UntagResource",
	}

	tests := make([]struct {
		name string
	}, len(stubs))
	for i, s := range stubs {
		tests[i].name = s
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doCognitoRequest(t, h, tt.name, map[string]any{
				"UserPoolId": "any",
				"Username":   "any",
			})
			assert.Equal(t, http.StatusOK, rec.Code, "action %s", tt.name)
		})
	}
}

func TestHandler_Name(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, "CognitoIDP", h.Name())
}

func TestHandler_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	ops := h.GetSupportedOperations()

	expected := []string{
		"CreateUserPool", "DescribeUserPool", "ListUserPools",
		"CreateUserPoolClient", "DescribeUserPoolClient",
		"SignUp", "ConfirmSignUp", "InitiateAuth", "AdminInitiateAuth",
		"AdminCreateUser", "AdminSetUserPassword", "AdminGetUser",
	}

	for _, op := range expected {
		assert.Contains(t, ops, op)
	}
}

func TestHandler_MatchPriority(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, 100, h.MatchPriority())
}

func TestHandler_RouteMatcher(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		path   string
		want   bool
	}{
		{
			name:   "matching_target",
			target: "AWSCognitoIdentityProviderService.CreateUserPool",
			path:   "/",
			want:   true,
		},
		{
			name: "matching_jwks_path",
			path: "/us-east-1_abc123/.well-known/jwks.json",
			want: true,
		},
		{
			name:   "non_matching",
			target: "AmazonSQS.SendMessage",
			path:   "/",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, tt.path, nil)

			if tt.target != "" {
				req.Header.Set("X-Amz-Target", tt.target)
			}

			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.want, h.RouteMatcher()(c))
		})
	}
}

func TestHandler_ExtractOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		path   string
		want   string
	}{
		{
			name:   "cognito_action",
			target: "AWSCognitoIdentityProviderService.CreateUserPool",
			path:   "/",
			want:   "CreateUserPool",
		},
		{
			name: "jwks_path",
			path: "/us-east-1_abc/.well-known/jwks.json",
			want: "GetJWKS",
		},
		{
			name: "unknown",
			path: "/",
			want: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, tt.path, nil)

			if tt.target != "" {
				req.Header.Set("X-Amz-Target", tt.target)
			}

			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.want, h.ExtractOperation(c))
		})
	}
}

func TestHandler_JWKS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *cognitoidp.Handler) string
		name         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(h *cognitoidp.Handler) string {
				rec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{"PoolName": "p"})
				var resp map[string]map[string]any
				_ = json.Unmarshal(rec.Body.Bytes(), &resp)

				return resp["UserPool"]["Id"].(string)
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"keys", "RSA", "RS256"},
		},
		{
			name: "pool_not_found",
			setup: func(_ *cognitoidp.Handler) string {
				return "us-east-1_nonexistent"
			},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			poolID := tt.setup(h)

			rec := doJWKSRequest(t, h, poolID)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, want := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), want)
			}
		})
	}
}

func TestHandler_UnknownAction(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doCognitoRequest(t, h, "NonExistentAction", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "UnknownOperationException")
}

func TestHandler_MissingTarget(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_ExtractResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		target  string
		body    string
		wantRes string
	}{
		{
			name:    "jwks_path_extracts_pool_id",
			path:    "/us-east-1_abc123/.well-known/jwks.json",
			wantRes: "us-east-1_abc123",
		},
		{
			name:    "body_user_pool_id",
			path:    "/",
			target:  "AWSCognitoIdentityProviderService.DescribeUserPool",
			body:    `{"UserPoolId":"us-east-1_poolXYZ"}`,
			wantRes: "us-east-1_poolXYZ",
		},
		{
			name:    "body_client_id",
			path:    "/",
			target:  "AWSCognitoIdentityProviderService.InitiateAuth",
			body:    `{"ClientId":"myclient"}`,
			wantRes: "myclient",
		},
		{
			name:    "body_username",
			path:    "/",
			target:  "AWSCognitoIdentityProviderService.SignUp",
			body:    `{"Username":"alice"}`,
			wantRes: "alice",
		},
		{
			name:    "empty_body",
			path:    "/",
			target:  "AWSCognitoIdentityProviderService.ListUserPools",
			body:    `{}`,
			wantRes: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			e := echo.New()

			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(http.MethodGet, tt.path, nil)
			}

			if tt.target != "" {
				req.Header.Set("X-Amz-Target", tt.target)
			}

			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.wantRes, h.ExtractResource(c))
		})
	}
}

func TestHandler_UnmarshalTypeError(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Sending a wrong type (array instead of string for PoolName) should return 400.
	rec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{
		"PoolName": []string{"not-a-string"},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidParameterException")
}

func TestHandler_ChaosOperations(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, "cognito-idp", h.ChaosServiceName())
	assert.Equal(t, h.GetSupportedOperations(), h.ChaosOperations())
	assert.Equal(t, []string{"us-east-1"}, h.ChaosRegions())
}

func TestReset(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	b := h.Backend

	poolRec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{"PoolName": "reset-pool"})
	require.Equal(t, http.StatusOK, poolRec.Code)

	var poolResp struct {
		UserPool struct {
			ID string `json:"Id,omitempty"`
		} `json:"UserPool"`
	}
	require.NoError(t, json.Unmarshal(poolRec.Body.Bytes(), &poolResp))
	poolID := poolResp.UserPool.ID
	assert.Equal(t, 1, b.UserPoolCount())

	client, err := b.CreateUserPoolClient(poolID, "reset-client")
	require.NoError(t, err)
	assert.Equal(t, 1, b.ClientCount())

	user, err := b.SignUp(client.ClientID, "reset-user", "Pass1234!", map[string]string{"email": "reset-user@x.com"})
	require.NoError(t, err)
	require.NoError(t, b.ConfirmSignUp(client.ClientID, "reset-user", user.ConfirmCode))

	authResult, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "reset-user", "Pass1234!")
	require.NoError(t, err)
	assert.Equal(t, 1, b.UserCount())

	// Populate attrVerificationCodes via the real API surface.
	_, _, _, err = b.GetUserAttributeVerificationCode(authResult.Tokens.AccessToken, "email")
	require.NoError(t, err)
	require.Equal(t, 1, b.AttrVerificationCodeCount())

	// Populate poolMfaConfigs. Setting MfaConfiguration "ON" also forces every
	// subsequent InitiateAuth in this pool through the MFA-challenge branch
	// (postCredentialCheckLocked), which populates mfaSessions below.
	require.NoError(t, b.SetUserPoolMfaConfigFull(poolID, cognitoidp.UserPoolMfaFullConfig{MfaConfiguration: "ON"}))
	require.Equal(t, 1, b.PoolMfaConfigCount())

	mfaResult, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "reset-user", "Pass1234!")
	require.NoError(t, err)
	require.NotEmpty(t, mfaResult.MFASession)
	require.Equal(t, 1, b.MFASessionCount())

	h.Reset()
	assert.Equal(t, 0, b.UserPoolCount())
	assert.Equal(t, 0, b.UserCount())
	assert.Equal(t, 0, b.ClientCount())
	assert.Equal(t, 0, b.AttrVerificationCodeCount(), "Reset must clear attrVerificationCodes")
	assert.Equal(t, 0, b.PoolMfaConfigCount(), "Reset must clear poolMfaConfigs")
	assert.Equal(t, 0, b.MFASessionCount(), "Reset must clear mfaSessions")
}

func TestMultipleResetCycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 3 {
		_ = i
		doCognitoRequest(t, h, "CreateUserPool", map[string]any{"PoolName": "cycle-pool"})
		h.Reset()
		assert.Equal(t, 0, h.Backend.UserPoolCount())
	}
}

func TestHandlerOpsPreBuilt(t *testing.T) {
	t.Parallel()

	// Ensure the handler works correctly with the cached dispatch table.
	h := newTestHandler(t)
	rec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{"PoolName": "ops-pool"})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestNonNilSlices(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	poolRec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{"PoolName": "nil-slices-pool"})
	var poolResp map[string]any
	require.NoError(t, json.Unmarshal(poolRec.Body.Bytes(), &poolResp))
	poolID := poolResp["UserPool"].(map[string]any)["Id"].(string)

	// Empty ListUsers must return [] not null.
	listRec := doCognitoRequest(t, h, "ListUsers", map[string]any{"UserPoolId": poolID})
	assert.Contains(t, listRec.Body.String(), `"Users":[]`)

	// Empty ListGroups must return [] not null.
	groupsRec := doCognitoRequest(t, h, "ListGroups", map[string]any{"UserPoolId": poolID})
	assert.Contains(t, groupsRec.Body.String(), `"Groups":[]`)

	// Empty ListUserPoolClients must return [] not null.
	clientsRec := doCognitoRequest(t, h, "ListUserPoolClients", map[string]any{"UserPoolId": poolID})
	assert.Contains(t, clientsRec.Body.String(), `"UserPoolClients":[]`)

	// Empty ListUserPools must return [] not null.
	h2 := newTestHandler(t)
	poolsRec := doCognitoRequest(t, h2, "ListUserPools", map[string]any{"MaxResults": 10})
	assert.Contains(t, poolsRec.Body.String(), `"UserPools":[]`)
}

func TestSeedHelpers(t *testing.T) {
	t.Parallel()

	backend := cognitoidp.NewInMemoryBackend("000000000000", "us-east-1", "http://localhost:8000")
	assert.Equal(t, 0, backend.UserPoolCount())

	backend.AddUserPoolInternal(&cognitoidp.UserPool{
		ID:   "us-east-1_TEST01",
		Name: "seed-pool",
		ARN:  "arn:aws:cognito-idp:us-east-1:000000000000:userpool/us-east-1_TEST01",
	})
	assert.Equal(t, 1, backend.UserPoolCount())

	backend.AddUserInternal(&cognitoidp.User{
		Sub:        "sub-123",
		Username:   "seed-user",
		UserPoolID: "us-east-1_TEST01",
		Status:     "CONFIRMED",
		Enabled:    true,
	})
	assert.Equal(t, 1, backend.UserCount())

	backend.AddUserPoolClientInternal(&cognitoidp.UserPoolClient{
		ClientID:   "client-123",
		ClientName: "seed-client",
		UserPoolID: "us-east-1_TEST01",
	})
	assert.Equal(t, 1, backend.ClientCount())
}

func TestExportCountHelpers(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, 0, h.Backend.UserPoolCount())
	assert.Equal(t, 0, h.Backend.UserCount())
	assert.Equal(t, 0, h.Backend.ClientCount())

	poolRec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{"PoolName": "count-pool"})
	var poolResp map[string]any
	require.NoError(t, json.Unmarshal(poolRec.Body.Bytes(), &poolResp))
	poolID := poolResp["UserPool"].(map[string]any)["Id"].(string)
	assert.Equal(t, 1, h.Backend.UserPoolCount())

	doCognitoRequest(t, h, "CreateUserPoolClient", map[string]any{
		"UserPoolId": poolID,
		"ClientName": "c1",
	})
	assert.Equal(t, 1, h.Backend.ClientCount())

	doCognitoRequest(t, h, "AdminCreateUser", map[string]any{
		"UserPoolId":        poolID,
		"Username":          "u1",
		"TemporaryPassword": "TempPass123!",
	})
	assert.Equal(t, 1, h.Backend.UserCount())
}

// provisionedLimitResp is the shared GetProvisionedLimit/UpdateProvisionedLimit
// wire response shape used by TestHandler_ProvisionedLimits.
type provisionedLimitResp struct {
	Limit struct {
		LimitDefinition struct {
			Attributes map[string]string `json:"Attributes,omitempty"`
			LimitClass string            `json:"LimitClass,omitempty"`
		} `json:"LimitDefinition"`
		FreeLimitValue        int32 `json:"FreeLimitValue"`
		ProvisionedLimitValue int32 `json:"ProvisionedLimitValue"`
	} `json:"Limit"`
}

// apiCategoryLimitDef builds a LimitDefinition wire payload for the given
// LimitClass/Category, keeping TestHandler_ProvisionedLimits's table under
// the line-length limit.
func apiCategoryLimitDef(limitClass, category string) map[string]any {
	return map[string]any{"LimitClass": limitClass, "Attributes": map[string]any{"Category": category}}
}

// TestHandler_ProvisionedLimits covers GetProvisionedLimit/UpdateProvisionedLimit,
// which are account-level (not per-user-pool) -- see provisioned_limits.go for
// the AWS-documented default values this asserts against.
func TestHandler_ProvisionedLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		limitDef map[string]any
		extra    map[string]any
		name     string
		op       string
		wantCode int
	}{
		{
			name:     "get_default_matches_documented_value",
			op:       "GetProvisionedLimit",
			limitDef: apiCategoryLimitDef("API_CATEGORY", "UserAuthentication"),
			wantCode: http.StatusOK,
		},
		{
			name:     "update_adjustable_category",
			op:       "UpdateProvisionedLimit",
			limitDef: apiCategoryLimitDef("API_CATEGORY", "UserAuthentication"),
			extra:    map[string]any{"RequestedLimitValue": 300},
			wantCode: http.StatusOK,
		},
		{
			name:     "update_non_adjustable_category_rejected",
			op:       "UpdateProvisionedLimit",
			limitDef: apiCategoryLimitDef("API_CATEGORY", "UserPoolRead"),
			extra:    map[string]any{"RequestedLimitValue": 100},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "unknown_category_rejected",
			op:       "GetProvisionedLimit",
			limitDef: apiCategoryLimitDef("API_CATEGORY", "NotARealCategory"),
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "wrong_limit_class_rejected",
			op:       "GetProvisionedLimit",
			limitDef: apiCategoryLimitDef("NOT_A_CLASS", "UserAuthentication"),
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "exceeds_account_max_rejected",
			op:       "UpdateProvisionedLimit",
			limitDef: apiCategoryLimitDef("API_CATEGORY", "UserAuthentication"),
			extra:    map[string]any{"RequestedLimitValue": 100000},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			body := map[string]any{"LimitDefinition": tt.limitDef}
			maps.Copy(body, tt.extra)

			rec := doCognitoRequest(t, h, tt.op, body)
			require.Equal(t, tt.wantCode, rec.Code, rec.Body.String())

			if tt.wantCode != http.StatusOK {
				return
			}

			var resp provisionedLimitResp
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "API_CATEGORY", resp.Limit.LimitDefinition.LimitClass)
			assert.EqualValues(t, 120, resp.Limit.FreeLimitValue)

			switch tt.name {
			case "get_default_matches_documented_value":
				assert.EqualValues(t, 120, resp.Limit.ProvisionedLimitValue)
			case "update_adjustable_category":
				assert.EqualValues(t, 300, resp.Limit.ProvisionedLimitValue)
			}
		})
	}
}

// TestHandler_ProvisionedLimits_RoundTrip verifies UpdateProvisionedLimit's
// new value is durably reflected by a subsequent GetProvisionedLimit call on
// the same handler instance (account-level state, not request-scoped).
func TestHandler_ProvisionedLimits_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	limitDef := apiCategoryLimitDef("API_CATEGORY", "UserCreation")

	updateRec := doCognitoRequest(t, h, "UpdateProvisionedLimit", map[string]any{
		"LimitDefinition":     limitDef,
		"RequestedLimitValue": 200,
	})
	require.Equal(t, http.StatusOK, updateRec.Code, updateRec.Body.String())

	getRec := doCognitoRequest(t, h, "GetProvisionedLimit", map[string]any{"LimitDefinition": limitDef})
	require.Equal(t, http.StatusOK, getRec.Code)

	var resp provisionedLimitResp
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &resp))
	assert.EqualValues(t, 200, resp.Limit.ProvisionedLimitValue)
	assert.EqualValues(t, 50, resp.Limit.FreeLimitValue)
}
