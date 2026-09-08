package iam_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awsmeta"
	"github.com/blackbirdworks/gopherstack/services/iam"
)

var (
	errKeyNotFound  = errors.New("access key not found")
	errUserNotFound = errors.New("user not found")
)

// mockEnforcementBackend implements iam.EnforcementBackend for testing. It
// also implements the middleware's optional permissionsBoundaryLookup
// capability so boundary-enforcement tests can exercise it, mirroring the
// real IAM backend without requiring every other service's own enforcement
// mock (which only implements the required interface) to change.
type mockEnforcementBackend struct {
	keyMap         map[string]string   // accessKeyID → userName
	policies       map[string][]string // userName/roleName → []policyDoc
	users          map[string]*iam.User
	userBoundaries map[string]string // userName → boundary policy doc
	roleBoundaries map[string]string // roleName → boundary policy doc
}

func newMockEnforcementBackend() *mockEnforcementBackend {
	return &mockEnforcementBackend{
		users:          make(map[string]*iam.User),
		keyMap:         make(map[string]string),
		policies:       make(map[string][]string),
		userBoundaries: make(map[string]string),
		roleBoundaries: make(map[string]string),
	}
}

func (m *mockEnforcementBackend) GetUserByAccessKeyID(accessKeyID string) (*iam.User, error) {
	userName, ok := m.keyMap[accessKeyID]
	if !ok {
		return nil, errKeyNotFound
	}

	u, ok := m.users[userName]
	if !ok {
		return nil, errUserNotFound
	}

	return u, nil
}

func (m *mockEnforcementBackend) GetPoliciesForUser(userName string) ([]string, error) {
	return m.policies[userName], nil
}

func (m *mockEnforcementBackend) GetPoliciesForRole(roleName string) ([]string, error) {
	return m.policies[roleName], nil
}

func (m *mockEnforcementBackend) PermissionsBoundaryDocForUser(userName string) string {
	return m.userBoundaries[userName]
}

func (m *mockEnforcementBackend) PermissionsBoundaryDocForRole(roleName string) string {
	return m.roleBoundaries[roleName]
}

// principalMiddleware seeds ctx with a pre-resolved principal, standing in for
// cli.go's real principalMiddleware (which runs STS/IAM PrincipalResolvers
// ahead of enforcement) so these unit tests can drive
// resolveSTSUserIdentityPolicies without spinning up a real STS backend.
func principalSeedingMiddleware(p *awsmeta.Principal) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			req := c.Request()
			meta := awsmeta.FromRequest(req, "us-east-1")
			meta.Principal = p
			ctx := awsmeta.Set(req.Context(), meta)
			c.SetRequest(req.WithContext(ctx))

			return next(c)
		}
	}
}

func TestEnforcementMiddleware(t *testing.T) {
	t.Parallel()

	allowS3All := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`
	allowIAMAll := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:*","Resource":"*"}]}`
	denyAll := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"*","Resource":"*"}]}`

	tests := []struct {
		setupBackend  func(*mockEnforcementBackend)
		headers       map[string]string
		name          string
		requestPath   string
		requestMethod string
		body          string
		wantStatus    int
	}{
		{
			name:          "no_credentials_passes",
			setupBackend:  func(_ *mockEnforcementBackend) {},
			requestPath:   "/",
			requestMethod: http.MethodGet,
			wantStatus:    http.StatusOK,
		},
		{
			name:          "unknown_key_passes",
			setupBackend:  func(_ *mockEnforcementBackend) {},
			requestPath:   "/",
			requestMethod: http.MethodGet,
			headers: map[string]string{
				"Authorization": "AWS4-HMAC-SHA256 Credential=UNKNOWN_KEY/20230101/us-east-1/s3/aws4_request",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "known_key_allow",
			setupBackend: func(b *mockEnforcementBackend) {
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIATEST1"] = "alice"
				b.policies["alice"] = []string{allowS3All}
			},
			requestPath:   "/my-bucket/key",
			requestMethod: http.MethodGet,
			headers: map[string]string{
				"Authorization": "AWS4-HMAC-SHA256 Credential=AKIATEST1/20230101/us-east-1/s3/aws4_request",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "known_key_implicit_deny",
			setupBackend: func(b *mockEnforcementBackend) {
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIATEST2"] = "alice"
				b.policies["alice"] = []string{allowIAMAll} // only IAM, not S3
			},
			requestPath:   "/my-bucket/key",
			requestMethod: http.MethodGet,
			headers: map[string]string{
				"Authorization": "AWS4-HMAC-SHA256 Credential=AKIATEST2/20230101/us-east-1/s3/aws4_request",
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "known_key_explicit_deny",
			setupBackend: func(b *mockEnforcementBackend) {
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIATEST3"] = "alice"
				b.policies["alice"] = []string{allowS3All, denyAll}
			},
			requestPath:   "/my-bucket/key",
			requestMethod: http.MethodGet,
			headers: map[string]string{
				"Authorization": "AWS4-HMAC-SHA256 Credential=AKIATEST3/20230101/us-east-1/s3/aws4_request",
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "dashboard_path_skipped",
			setupBackend: func(b *mockEnforcementBackend) {
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIATEST4"] = "alice"
				b.policies["alice"] = []string{} // no policies
			},
			requestPath:   "/dashboard/iam",
			requestMethod: http.MethodGet,
			headers: map[string]string{
				"Authorization": "AWS4-HMAC-SHA256 Credential=AKIATEST4/20230101/us-east-1/s3/aws4_request",
			},
			wantStatus: http.StatusOK, // skipped — dashboard path
		},
		{
			name: "health_path_skipped",
			setupBackend: func(b *mockEnforcementBackend) {
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIATEST5"] = "alice"
				b.policies["alice"] = []string{} // no policies
			},
			requestPath:   "/_gopherstack/health",
			requestMethod: http.MethodGet,
			headers: map[string]string{
				"Authorization": "AWS4-HMAC-SHA256 Credential=AKIATEST5/20230101/us-east-1/s3/aws4_request",
			},
			wantStatus: http.StatusOK, // skipped — internal path
		},
		{
			name: "no_policies_implicit_deny",
			setupBackend: func(b *mockEnforcementBackend) {
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIATEST6"] = "alice"
				b.policies["alice"] = []string{}
			},
			requestPath:   "/my-bucket/key",
			requestMethod: http.MethodPut,
			headers: map[string]string{
				"Authorization": "AWS4-HMAC-SHA256 Credential=AKIATEST6/20230101/us-east-1/s3/aws4_request",
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "jsonrpc_implicit_deny_returns_400_bad_request",
			setupBackend: func(b *mockEnforcementBackend) {
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIAJSONRPC"] = "alice"
				b.policies["alice"] = []string{}
			},
			requestPath:   "/",
			requestMethod: http.MethodPost,
			headers: map[string]string{
				"Authorization": "AWS4-HMAC-SHA256 Credential=AKIAJSONRPC/20230101/us-east-1/dynamodb/aws4_request",
				"X-Amz-Target":  "DynamoDB_20120810.PutItem",
				"Content-Type":  "application/x-amz-json-1.0",
			},
			body:       `{"TableName":"orders"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "condition_requested_region_match",
			setupBackend: func(b *mockEnforcementBackend) {
				policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*",` +
					`"Resource":"*","Condition":{"StringEquals":{"aws:RequestedRegion":"us-east-1"}}}]}`
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIAREGION1"] = "alice"
				b.policies["alice"] = []string{policy}
			},
			requestPath:   "/my-bucket/key",
			requestMethod: http.MethodGet,
			headers: map[string]string{
				"Authorization": "AWS4-HMAC-SHA256 Credential=AKIAREGION1/20230101/us-east-1/s3/aws4_request",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "condition_principal_account_match",
			setupBackend: func(b *mockEnforcementBackend) {
				policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*",` +
					`"Resource":"*","Condition":{"StringEquals":{"aws:PrincipalAccount":"123456789012"}}}]}`
				b.users["alice"] = &iam.User{UserName: "alice", Arn: "arn:aws:iam::123456789012:user/alice"}
				b.keyMap["AKIAACCT1"] = "alice"
				b.policies["alice"] = []string{policy}
			},
			requestPath:   "/my-bucket/key",
			requestMethod: http.MethodGet,
			headers: map[string]string{
				"Authorization": "AWS4-HMAC-SHA256 Credential=AKIAACCT1/20230101/us-east-1/s3/aws4_request",
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newMockEnforcementBackend()
			tt.setupBackend(backend)

			e := echo.New()
			e.Use(iam.EnforcementMiddleware(backend))
			e.Any("/*", func(c *echo.Context) error {
				return c.String(http.StatusOK, "ok")
			})

			reqBody := strings.NewReader(tt.body)
			req := httptest.NewRequest(tt.requestMethod, tt.requestPath, reqBody)

			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code, "status code mismatch")
		})
	}
}

func TestExtractAccessKeyIDFromRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		authorization string
		want          string
	}{
		{
			name: "valid_credential",
			authorization: "AWS4-HMAC-SHA256 Credential=AKIA1234/20230101/us-east-1/s3/aws4_request," +
				" SignedHeaders=host, Signature=xyz",
			want: "AKIA1234",
		},
		{
			name:          "empty",
			authorization: "",
			want:          "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}

			got := iam.ExtractAccessKeyID(req)
			require.Equal(t, tt.want, got)
		})
	}
}

// mockActionExtractor implements iam.ActionExtractor for testing.
type mockActionExtractor struct {
	action string
}

func (m *mockActionExtractor) IAMAction(_ *http.Request) string {
	return m.action
}

func TestEnforcementMiddleware_ActionExtractors(t *testing.T) {
	t.Parallel()

	allowLambda := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"lambda:*","Resource":"*"}]}`

	tests := []struct {
		setupBackend func(*mockEnforcementBackend)
		extractor    *mockActionExtractor
		name         string
		requestPath  string
		wantStatus   int
	}{
		{
			name: "extractor_returns_allowed_action",
			setupBackend: func(b *mockEnforcementBackend) {
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIAEXT1"] = "alice"
				b.policies["alice"] = []string{allowLambda}
			},
			extractor:   &mockActionExtractor{action: "lambda:InvokeFunction"},
			requestPath: "/2015-03-31/functions/my-func/invocations",
			wantStatus:  http.StatusOK,
		},
		{
			name: "extractor_returns_denied_action",
			setupBackend: func(b *mockEnforcementBackend) {
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIAEXT2"] = "alice"
				b.policies["alice"] = []string{allowLambda} // lambda allowed, not s3
			},
			extractor:   &mockActionExtractor{action: "s3:GetObject"}, // overrides to s3 → denied
			requestPath: "/2015-03-31/functions/my-func/invocations",
			wantStatus:  http.StatusForbidden,
		},
		{
			name: "extractor_returns_empty_passes_through",
			setupBackend: func(b *mockEnforcementBackend) {
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIAEXT3"] = "alice"
				b.policies["alice"] = []string{} // no policies
			},
			extractor:   &mockActionExtractor{action: ""},            // empty → no enforcement
			requestPath: "/2015-03-31/functions/my-func/invocations", // Lambda path excluded from S3 detection
			wantStatus:  http.StatusOK,                               // passes through when action unknown
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newMockEnforcementBackend()
			tt.setupBackend(backend)

			cfg := iam.EnforcementConfig{
				ActionExtractors: []iam.ActionExtractor{tt.extractor},
			}

			e := echo.New()
			e.Use(iam.EnforcementMiddleware(backend, cfg))
			e.Any("/*", func(c *echo.Context) error {
				return c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(http.MethodPost, tt.requestPath, strings.NewReader(""))
			req.Header.Set(
				"Authorization",
				"AWS4-HMAC-SHA256 Credential="+getKey(backend)+"/20230101/us-east-1/lambda/aws4_request",
			)

			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// getKey extracts the access key set by the setup function.
func getKey(b *mockEnforcementBackend) string {
	for k := range b.keyMap {
		return k
	}

	return "UNKNOWN"
}

func TestEnforcementMiddleware_ResourcePolicyProviders(t *testing.T) {
	t.Parallel()

	allowResourcePolicy := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Allow","Principal":"*","Action":["s3:GetObject","lambda:InvokeFunction"],"Resource":"*"}]
	}`
	denyResourcePolicy := `{
		"Version":"2012-10-17",
		"Statement":[{"Effect":"Deny","Principal":"*","Action":"*","Resource":"*"}]
	}`

	tests := []struct {
		setupBackend func(*mockEnforcementBackend)
		provider     *mockResourceProvider
		name         string
		requestPath  string
		method       string
		wantStatus   int
	}{
		{
			name: "resource_policy_allows_when_user_has_no_policy",
			setupBackend: func(b *mockEnforcementBackend) {
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIARES1"] = "alice"
				b.policies["alice"] = []string{}
			},
			provider: &mockResourceProvider{
				policies: map[string]string{
					"arn:aws:s3:::my-bucket/obj": allowResourcePolicy,
				},
			},
			requestPath: "/my-bucket/obj",
			method:      http.MethodGet,
			wantStatus:  http.StatusOK,
		},
		{
			name: "resource_policy_explicit_deny_overrides_allow",
			setupBackend: func(b *mockEnforcementBackend) {
				allowS3 := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIARES2"] = "alice"
				b.policies["alice"] = []string{allowS3}
			},
			provider: &mockResourceProvider{
				policies: map[string]string{
					"arn:aws:s3:::denied-bucket/obj": denyResourcePolicy,
				},
			},
			requestPath: "/denied-bucket/obj",
			method:      http.MethodGet,
			wantStatus:  http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newMockEnforcementBackend()
			tt.setupBackend(backend)

			cfg := iam.EnforcementConfig{
				ResourceProviders: []iam.ResourcePolicyProvider{tt.provider},
			}

			e := echo.New()
			e.Use(iam.EnforcementMiddleware(backend, cfg))
			e.Any("/*", func(c *echo.Context) error {
				return c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(tt.method, tt.requestPath, strings.NewReader(""))
			req.Header.Set(
				"Authorization",
				"AWS4-HMAC-SHA256 Credential="+getKey(backend)+"/20230101/us-east-1/s3/aws4_request",
			)

			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestEnforcementMiddleware_STSUserPrincipal verifies gopherstack-s982:
// a Kind=User principal minted by STS (GetSessionToken/GetFederationToken/
// GetDelegatedAccessToken, which keep the caller's own identity instead of
// assuming a role) is not silently allowed through as an unrecognized/dummy
// key just because its ASIA access key ID is absent from IAM's own user
// table. principalSeedingMiddleware stands in for cli.go's real
// principalMiddleware, which would have already resolved this Principal via
// STS's ResolvePrincipal before enforcement runs.
func TestEnforcementMiddleware_STSUserPrincipal(t *testing.T) {
	t.Parallel()

	allowS3All := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`

	tests := []struct {
		principal    *awsmeta.Principal
		setupBackend func(*mockEnforcementBackend)
		name         string
		wantStatus   int
	}{
		{
			name: "get_session_token_underlying_user_policy_allows",
			principal: &awsmeta.Principal{
				Kind: awsmeta.PrincipalKindUser,
				Arn:  "arn:aws:iam::123456789012:user/alice",
			},
			setupBackend: func(b *mockEnforcementBackend) {
				b.policies["alice"] = []string{allowS3All}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "get_session_token_underlying_user_no_policy_implicit_deny",
			principal: &awsmeta.Principal{
				Kind: awsmeta.PrincipalKindUser,
				Arn:  "arn:aws:iam::123456789012:user/bob",
			},
			setupBackend: func(_ *mockEnforcementBackend) {},
			wantStatus:   http.StatusForbidden,
		},
		{
			name: "get_federation_token_no_policy_record_implicit_deny",
			principal: &awsmeta.Principal{
				Kind: awsmeta.PrincipalKindUser,
				Arn:  "arn:aws:sts::123456789012:federated-user/feduser",
			},
			setupBackend: func(_ *mockEnforcementBackend) {},
			wantStatus:   http.StatusForbidden,
		},
		{
			name: "root_session_left_unenforced",
			principal: &awsmeta.Principal{
				Kind: awsmeta.PrincipalKindUser,
				Arn:  "arn:aws:iam::000000000000:root",
			},
			setupBackend: func(_ *mockEnforcementBackend) {},
			wantStatus:   http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newMockEnforcementBackend()
			tt.setupBackend(backend)

			e := echo.New()
			e.Use(principalSeedingMiddleware(tt.principal))
			e.Use(iam.EnforcementMiddleware(backend))
			e.Any("/*", func(c *echo.Context) error {
				return c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(http.MethodGet, "/my-bucket/key", strings.NewReader(""))
			req.Header.Set(
				"Authorization",
				"AWS4-HMAC-SHA256 Credential=ASIAFAKESESSIONKEY/20230101/us-east-1/s3/aws4_request",
			)

			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestEnforcementMiddleware_PermissionBoundary verifies gopherstack-7gnj: a
// permission boundary is consulted by the live enforcement path, not just by
// SimulatePrincipalPolicy. Per the IAM User Guide's "Permissions boundaries for
// IAM entities" page, an entity's permissions boundary lets it act only within
// what both its identity-based policies and its permissions boundary allow.
func TestEnforcementMiddleware_PermissionBoundary(t *testing.T) {
	t.Parallel()

	allowS3All := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`
	allowIAMOnlyBoundary := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iam:*","Resource":"*"}]}`
	allowS3Boundary := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`
	denyAllBoundary := `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"*","Resource":"*"}]}`

	tests := []struct {
		setupBackend func(*mockEnforcementBackend)
		name         string
		wantStatus   int
	}{
		{
			name: "identity_allow_no_boundary_still_allowed",
			setupBackend: func(b *mockEnforcementBackend) {
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIABOUND1"] = "alice"
				b.policies["alice"] = []string{allowS3All}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "identity_allow_but_boundary_does_not_cover_action_denied",
			setupBackend: func(b *mockEnforcementBackend) {
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIABOUND1"] = "alice"
				b.policies["alice"] = []string{allowS3All}
				b.userBoundaries["alice"] = allowIAMOnlyBoundary
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "identity_allow_and_boundary_covers_action_allowed",
			setupBackend: func(b *mockEnforcementBackend) {
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIABOUND1"] = "alice"
				b.policies["alice"] = []string{allowS3All}
				b.userBoundaries["alice"] = allowS3Boundary
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "identity_allow_but_boundary_explicit_deny_denied",
			setupBackend: func(b *mockEnforcementBackend) {
				b.users["alice"] = &iam.User{UserName: "alice"}
				b.keyMap["AKIABOUND1"] = "alice"
				b.policies["alice"] = []string{allowS3All}
				b.userBoundaries["alice"] = denyAllBoundary
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newMockEnforcementBackend()
			tt.setupBackend(backend)

			e := echo.New()
			e.Use(iam.EnforcementMiddleware(backend))
			e.Any("/*", func(c *echo.Context) error {
				return c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(http.MethodGet, "/my-bucket/key", strings.NewReader(""))
			req.Header.Set(
				"Authorization",
				"AWS4-HMAC-SHA256 Credential=AKIABOUND1/20230101/us-east-1/s3/aws4_request",
			)

			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
