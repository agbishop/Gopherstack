package iam_test

import (
	"encoding/base64"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	iamsdk "github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iam"
)

func TestGetAccountAuthorizationDetails_GroupList(t *testing.T) {
	t.Parallel()

	e := echo.New()
	h, b := newTestHandler(t)

	// Create users, groups, memberships via backend to avoid ordering dependencies.
	_, err := b.CreateUser("alice", "/", "")
	require.NoError(t, err)
	_, err = b.CreateUser("bob", "/", "")
	require.NoError(t, err)
	_, err = b.CreateGroup("admins", "/")
	require.NoError(t, err)
	_, err = b.CreateGroup("devs", "/")
	require.NoError(t, err)
	require.NoError(t, b.AddUserToGroup("admins", "alice"))
	require.NoError(t, b.AddUserToGroup("devs", "alice"))
	require.NoError(t, b.AddUserToGroup("devs", "bob"))

	rec := iamCall(t, e, h, "GetAccountAuthorizationDetails", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	type userEntry struct {
		UserName  string   `xml:"UserName"`
		GroupList []string `xml:"GroupList>member"`
	}
	var resp struct {
		XMLName        xml.Name    `xml:"GetAccountAuthorizationDetailsResponse"`
		UserDetailList []userEntry `xml:"GetAccountAuthorizationDetailsResult>UserDetailList>member"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))

	userGroups := make(map[string][]string)
	for _, u := range resp.UserDetailList {
		userGroups[u.UserName] = u.GroupList
	}

	assert.ElementsMatch(t, []string{"admins", "devs"}, userGroups["alice"],
		"alice should be in both groups")
	assert.ElementsMatch(t, []string{"devs"}, userGroups["bob"],
		"bob should be in devs only")
}

func TestGetAccountAuthorizationDetails_GroupList_Empty(t *testing.T) {
	t.Parallel()

	e := echo.New()
	h, b := newTestHandler(t)

	_, err := b.CreateUser("solo", "/", "")
	require.NoError(t, err)

	rec := iamCall(t, e, h, "GetAccountAuthorizationDetails", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	type userEntry2 struct {
		UserName  string   `xml:"UserName"`
		GroupList []string `xml:"GroupList>member"`
	}
	var resp struct {
		XMLName        xml.Name     `xml:"GetAccountAuthorizationDetailsResponse"`
		UserDetailList []userEntry2 `xml:"GetAccountAuthorizationDetailsResult>UserDetailList>member"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))

	for _, u := range resp.UserDetailList {
		if u.UserName == "solo" {
			assert.Empty(t, u.GroupList, "user with no memberships should have empty GroupList")
		}
	}
}

// Test_GetAccountAuthorizationDetails_PolicyVersions verifies that
// GetAccountAuthorizationDetails reports every real stored version of a
// managed policy (via StorageBackend.ListPolicyVersions), not a single
// hardcoded fake "v1" entry that ignored CreatePolicyVersion/SetDefaultPolicyVersion
// state entirely.

func Test_GetAccountAuthorizationDetails_PolicyVersions(t *testing.T) {
	t.Parallel()

	e := echo.New()
	h, b := newTestHandler(t)

	pol, err := b.CreatePolicy(
		"MultiVersionPolicy", "/",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`,
	)
	require.NoError(t, err)

	v2, err := b.CreatePolicyVersion(
		pol.Arn,
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`,
		true, // set as default
	)
	require.NoError(t, err)

	req := iamRequest("GetAccountAuthorizationDetails", nil)
	rec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req, rec)))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp iam.GetAccountAuthorizationDetailsResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))

	require.Len(t, resp.GetAccountAuthorizationDetailsResult.Policies, 1)
	managed := resp.GetAccountAuthorizationDetailsResult.Policies[0]

	require.Len(t, managed.PolicyVersionList, 2, "must report both v1 and v2, not a fake single version")

	byVersion := map[string]iam.PolicyVersionXML{}
	for _, v := range managed.PolicyVersionList {
		byVersion[v.VersionID] = v
	}

	v1XML, ok := byVersion["v1"]
	require.True(t, ok, "v1 must be present")
	assert.False(t, v1XML.IsDefaultVersion, "v1 is no longer default after SetAsDefault on v2")

	v2XML, ok := byVersion[v2.VersionID]
	require.True(t, ok, "%s must be present", v2.VersionID)
	assert.True(t, v2XML.IsDefaultVersion)
	// Document must be percent-encoded, matching GetPolicyVersion's documented behavior.
	assert.Contains(t, v2XML.Document, "s3%3A%2A")
}

func TestCredentialReport_Columns(t *testing.T) {
	t.Parallel()

	requiredColumns := []string{
		"user",
		"arn",
		"user_creation_time",
		"password_enabled",
		"password_last_used",
		"password_last_changed",
		"password_next_rotation",
		"mfa_active",
		"access_key_1_active",
		"access_key_1_last_rotated",
		"access_key_1_last_used_date",
		"access_key_1_last_used_region",
		"access_key_1_last_used_service",
		"access_key_2_active",
		"access_key_2_last_rotated",
		"access_key_2_last_used_date",
		"access_key_2_last_used_region",
		"access_key_2_last_used_service",
		"cert_1_active",
		"cert_1_last_rotated",
		"cert_2_active",
		"cert_2_last_rotated",
	}

	tests := []struct {
		setup func(b *iam.InMemoryBackend)
		name  string
	}{
		{
			name:  "empty_account",
			setup: func(_ *iam.InMemoryBackend) {},
		},
		{
			name: "account_with_user",
			setup: func(b *iam.InMemoryBackend) {
				_, err := b.CreateUser("alice", "/", "")
				require.NoError(t, err)
			},
		},
		{
			name: "user_with_access_key",
			setup: func(b *iam.InMemoryBackend) {
				_, err := b.CreateUser("bob", "/", "")
				require.NoError(t, err)
				_, err = b.CreateAccessKey("bob")
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := iam.NewInMemoryBackend()
			tt.setup(b)

			h := iam.NewHandler(b)
			e := echo.New()

			req := iamRequest("GenerateCredentialReport", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			require.NoError(t, h.Handler()(c))
			assert.Equal(t, http.StatusOK, rec.Code)

			req = iamRequest("GetCredentialReport", nil)
			rec = httptest.NewRecorder()
			c = e.NewContext(req, rec)
			require.NoError(t, h.Handler()(c))
			assert.Equal(t, http.StatusOK, rec.Code)

			var resp struct {
				XMLName xml.Name `xml:"GetCredentialReportResponse"`
				Result  struct {
					Content      string `xml:"Content"`
					ReportFormat string `xml:"ReportFormat"`
				} `xml:"GetCredentialReportResult"`
			}

			require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "text/csv", resp.Result.ReportFormat)
			assert.NotEmpty(t, resp.Result.Content, "Content must not be empty")

			decoded, err := base64.StdEncoding.DecodeString(resp.Result.Content)
			require.NoError(t, err, "Content must be valid base64")

			headerLine, _, _ := strings.Cut(string(decoded), "\n")
			cols := strings.Split(headerLine, ",")

			assert.Len(t, cols, len(requiredColumns),
				"credential report must have exactly %d columns", len(requiredColumns))

			for i, want := range requiredColumns {
				if i < len(cols) {
					assert.Equal(t, want, cols[i],
						"column %d must be %q, got %q", i, want, cols[i])
				}
			}
		})
	}
}

func TestHandler_GenerateOrganizationsAccessReport_ReturnsJobID(t *testing.T) {
	t.Parallel()

	e := echo.New()
	h, _ := newTestHandler(t)

	req := iamRequest("GenerateOrganizationsAccessReport", map[string]string{
		"EntityPath": "o-example12345678901/r-xy12/ou-xy12-abcdefgh",
	})
	rec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "JobId")
}

func TestHandler_GetOrganizationsAccessReport_CompletedStatus(t *testing.T) {
	t.Parallel()

	e := echo.New()
	h, b := newTestHandler(t)

	// Generate a report and capture the JobId.
	jobID := b.GenerateOrganizationsAccessReport("o-example12345678901/r-xy12/ou-xy12-abcdefgh")
	require.NotEmpty(t, jobID)

	req := iamRequest("GetOrganizationsAccessReport", map[string]string{"JobId": jobID})
	rec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "COMPLETED", "report must reach COMPLETED status")
}

func TestHandler_GetCredentialReport_ResponseFormat(t *testing.T) {
	t.Parallel()

	e := echo.New()
	h, b := newTestHandler(t)
	_, _ = b.CreateUser("report-handler-user", "/", "")

	req := iamRequest("GetCredentialReport", nil)
	rec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "GetCredentialReportResult")
	assert.Contains(t, body, "Content")
	assert.Contains(t, body, "ReportFormat")
	assert.Contains(t, body, "GeneratedTime")
	assert.Contains(t, body, "text/csv")
}

func TestHandler_GenerateAndGetCredentialReport(t *testing.T) {
	t.Parallel()

	e := echo.New()
	h, b := newTestHandler(t)
	_, _ = b.CreateUser("gen-report-user", "/", "")
	_, _ = b.CreateAccessKey("gen-report-user")

	// Generate.
	req := iamRequest("GenerateCredentialReport", nil)
	rec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req, rec)))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "COMPLETE")

	// Get.
	req2 := iamRequest("GetCredentialReport", nil)
	rec2 := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req2, rec2)))
	require.Equal(t, http.StatusOK, rec2.Code)

	body := rec2.Body.String()
	assert.Contains(t, body, "text/csv")
	assert.Contains(t, body, "GeneratedTime")
}

func TestHandler_CredentialReport_Generated(t *testing.T) {
	t.Parallel()

	e := echo.New()
	h, b := newTestHandler(t)
	_, _ = b.CreateUser("alice", "/", "")

	// GenerateCredentialReport.
	req := iamRequest("GenerateCredentialReport", map[string]string{})
	rec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusOK, rec.Code)

	// GetCredentialReport.
	req2 := iamRequest("GetCredentialReport", map[string]string{})
	rec2 := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req2, rec2)))
	assert.Equal(t, http.StatusOK, rec2.Code)

	body := rec2.Body.String()
	assert.Contains(t, body, "ReportFormat", "response must include format field")
}

func TestHandler_GetAccountSummary(t *testing.T) {
	t.Parallel()

	e := echo.New()
	h, b := newTestHandler(t)

	_, _ = b.CreateUser("alice", "/", "")
	_, _ = b.CreateUser("bob", "/", "")
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`
	_, _ = b.CreateRole("R", "/", doc, "")
	_, _ = b.CreateGroup("G", "/")

	req := iamRequest("GetAccountSummary", map[string]string{})
	rec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "SummaryMap")
}

func TestHandler_GetAccountAuthorizationDetails(t *testing.T) {
	t.Parallel()

	e := echo.New()
	h, b := newTestHandler(t)
	_, _ = b.CreateUser("alice", "/", "")
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`
	_, _ = b.CreateRole("R", "/", doc, "")

	req := iamRequest("GetAccountAuthorizationDetails", map[string]string{})
	rec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "alice")
}

func TestGetAccountAuthorizationDetails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*iam.InMemoryBackend)
		name         string
		wantUsers    int
		wantGroups   int
		wantRoles    int
		wantPolicies int
	}{
		{
			name:  "empty_account",
			setup: func(_ *iam.InMemoryBackend) {},
		},
		{
			name: "populated_account",
			setup: func(b *iam.InMemoryBackend) {
				_, _ = b.CreateUser("alice", "/", "")
				_, _ = b.CreateUser("bob", "/", "")
				_, _ = b.CreateGroup("admins", "/")
				_, _ = b.CreateRole(
					"my-role",
					"/",
					`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`,
					"",
				)
				pol, _ := b.CreatePolicy(
					"MyPolicy",
					"/",
					`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`,
				)
				_ = b.AttachUserPolicy("alice", pol.Arn)
				_ = b.PutUserPolicy(
					"alice",
					"InlineP",
					`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`,
				)
				_ = b.PutRolePolicy(
					"my-role",
					"InlineR",
					`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`,
				)
			},
			wantUsers:    2,
			wantGroups:   1,
			wantRoles:    1,
			wantPolicies: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			h, b := newTestHandler(t)
			tt.setup(b)

			req := iamRequest("GetAccountAuthorizationDetails", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := h.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)

			var resp iam.GetAccountAuthorizationDetailsResponse
			require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))

			assert.Len(t, resp.GetAccountAuthorizationDetailsResult.UserDetailList, tt.wantUsers)
			assert.Len(t, resp.GetAccountAuthorizationDetailsResult.GroupDetailList, tt.wantGroups)
			assert.Len(t, resp.GetAccountAuthorizationDetailsResult.RoleDetailList, tt.wantRoles)
			assert.Len(t, resp.GetAccountAuthorizationDetailsResult.Policies, tt.wantPolicies)
			assert.False(t, resp.GetAccountAuthorizationDetailsResult.IsTruncated)
		})
	}
}

func TestGetAccountAuthorizationDetails_InlinePoliciesIncluded(t *testing.T) {
	t.Parallel()

	e := echo.New()
	h, b := newTestHandler(t)

	_, _ = b.CreateUser("alice", "/", "")
	_ = b.PutUserPolicy(
		"alice",
		"MyInline",
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`,
	)

	req := iamRequest("GetAccountAuthorizationDetails", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp iam.GetAccountAuthorizationDetailsResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))

	require.Len(t, resp.GetAccountAuthorizationDetailsResult.UserDetailList, 1)
	user := resp.GetAccountAuthorizationDetailsResult.UserDetailList[0]
	assert.Equal(t, "alice", user.UserName)
	require.Len(t, user.UserPolicyList, 1)
	assert.Equal(t, "MyInline", user.UserPolicyList[0].PolicyName)
}

func TestSimulatePrincipalPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setup          func(*iam.InMemoryBackend)
		params         map[string]string
		wantDecision   string
		wantHTTPStatus int
		wantErr        bool
	}{
		{
			name: "user_with_allow_policy",
			setup: func(b *iam.InMemoryBackend) {
				_, _ = b.CreateUser("alice", "/", "")
				pol, _ := b.CreatePolicy("AllowS3", "/", allowS3GetObjectPolicy)
				_ = b.AttachUserPolicy("alice", pol.Arn)
			},
			params: map[string]string{
				"PolicySourceArn":       "arn:aws:iam::000000000000:user/alice",
				"ActionNames.member.1":  "s3:GetObject",
				"ResourceArns.member.1": "arn:aws:s3:::my-bucket/key",
			},
			wantDecision:   "allowed",
			wantHTTPStatus: http.StatusOK,
		},
		{
			name: "user_implicit_deny",
			setup: func(b *iam.InMemoryBackend) {
				_, _ = b.CreateUser("bob", "/", "")
			},
			params: map[string]string{
				"PolicySourceArn":      "arn:aws:iam::000000000000:user/bob",
				"ActionNames.member.1": "s3:DeleteObject",
			},
			wantDecision:   "implicitDeny",
			wantHTTPStatus: http.StatusOK,
		},
		{
			name: "role_with_inline_deny",
			setup: func(b *iam.InMemoryBackend) {
				_, _ = b.CreateRole("my-role", "/", `{}`, "")
				_ = b.PutRolePolicy("my-role", "DenyAll", denyAllPolicy)
			},
			params: map[string]string{
				"PolicySourceArn":      "arn:aws:iam::000000000000:role/my-role",
				"ActionNames.member.1": "s3:GetObject",
			},
			wantDecision:   "explicitDeny",
			wantHTTPStatus: http.StatusOK,
		},
		{
			name:  "user_not_found",
			setup: func(_ *iam.InMemoryBackend) {},
			params: map[string]string{
				"PolicySourceArn":      "arn:aws:iam::000000000000:user/nonexistent",
				"ActionNames.member.1": "s3:GetObject",
			},
			wantHTTPStatus: http.StatusNotFound, // NoSuchEntity is 404 on real AWS IAM.
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			h, b := newTestHandler(t)
			tt.setup(b)

			req := iamRequest("SimulatePrincipalPolicy", tt.params)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := h.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, tt.wantHTTPStatus, rec.Code)

			if !tt.wantErr {
				var resp iam.SimulatePrincipalPolicyResponse
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))

				require.NotEmpty(t, resp.SimulatePrincipalPolicyResult.EvaluationResults)
				assert.Equal(t, tt.wantDecision, resp.SimulatePrincipalPolicyResult.EvaluationResults[0].EvalDecision)
			}
		})
	}
}

func TestGenerateAndGetCredentialReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(*iam.InMemoryBackend)
		check  func(*testing.T, *httptest.ResponseRecorder)
		name   string
		action string
	}{
		{
			name:   "GenerateCredentialReport_returns_complete",
			setup:  func(_ *iam.InMemoryBackend) {},
			action: "GenerateCredentialReport",
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()

				var resp iam.GenerateCredentialReportResponse
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, "COMPLETE", resp.GenerateCredentialReportResult.State)
			},
		},
		{
			name:   "GetCredentialReport_empty",
			setup:  func(_ *iam.InMemoryBackend) {},
			action: "GetCredentialReport",
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()

				var resp iam.GetCredentialReportResponse
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))

				result := resp.GetCredentialReportResult
				assert.Equal(t, "text/csv", result.ReportFormat)
				assert.NotEmpty(t, result.Content)
				assert.NotEmpty(t, result.GeneratedTime)
			},
		},
		{
			name: "GetCredentialReport_with_users",
			setup: func(b *iam.InMemoryBackend) {
				_, _ = b.CreateUser("alice", "/", "")
				_, _ = b.CreateUser("bob", "/", "")
			},
			action: "GetCredentialReport",
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()

				var resp iam.GetCredentialReportResponse
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))

				content := resp.GetCredentialReportResult.Content
				assert.NotEmpty(t, content)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			h, b := newTestHandler(t)
			tt.setup(b)

			req := iamRequest(tt.action, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := h.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)

			tt.check(t, rec)
		})
	}
}

// TestGetAccountAuthorizationDetails_PermissionsBoundaryAndTags_RealClient is a
// regression test for gopherstack-21my: UserDetailXML and RoleDetailXML (the
// per-entity item shapes for GetAccountAuthorizationDetails) omitted
// PermissionsBoundary and Tags entirely, even though the real UserDetail/
// RoleDetail deserializer (iam@v1.58.1 deserializers.go,
// awsAwsquery_deserializeDocumentUserDetail / ...RoleDetail) reads both, and
// even though this same service's sibling ops -- toUserXML (GetUser/ListUsers)
// and toRoleXML (GetRole/ListRoles) -- already emit them correctly from the
// same backing User.PermissionsBoundary/Tags and Role.PermissionsBoundary/Tags
// fields. Right entity count, permanently blank PermissionsBoundary/Tags for
// every user and role in the account-wide report. Seeds two distinguishable
// users and two distinguishable roles.
func TestGetAccountAuthorizationDetails_PermissionsBoundaryAndTags_RealClient(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	client := newTestIAMClient(t, h)

	const boundaryArn1 = "arn:aws:iam::123456789012:policy/user-boundary"
	const boundaryArn2 = "arn:aws:iam::123456789012:policy/role-boundary"

	_, err := client.CreateUser(t.Context(), &iamsdk.CreateUserInput{
		UserName:            aws.String("gaad-user-1"),
		PermissionsBoundary: aws.String(boundaryArn1),
		Tags:                []types.Tag{{Key: aws.String("team"), Value: aws.String("platform")}},
	})
	require.NoError(t, err)

	_, err = client.CreateRole(t.Context(), &iamsdk.CreateRoleInput{
		RoleName:                 aws.String("gaad-role-1"),
		AssumeRolePolicyDocument: aws.String(testPolicyDoc),
		PermissionsBoundary:      aws.String(boundaryArn2),
		Tags:                     []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
	})
	require.NoError(t, err)

	out, err := client.GetAccountAuthorizationDetails(t.Context(), &iamsdk.GetAccountAuthorizationDetailsInput{})
	require.NoError(t, err)

	userByName := make(map[string]types.UserDetail, len(out.UserDetailList))
	for _, u := range out.UserDetailList {
		userByName[aws.ToString(u.UserName)] = u
	}

	roleByName := make(map[string]types.RoleDetail, len(out.RoleDetailList))
	for _, r := range out.RoleDetailList {
		roleByName[aws.ToString(r.RoleName)] = r
	}

	u1, ok := userByName["gaad-user-1"]
	require.True(t, ok)
	require.NotNil(t, u1.PermissionsBoundary)
	require.NotNil(t, u1.PermissionsBoundary.PermissionsBoundaryArn)
	assert.Equal(t, boundaryArn1, *u1.PermissionsBoundary.PermissionsBoundaryArn)
	require.Len(t, u1.Tags, 1)
	assert.Equal(t, "team", aws.ToString(u1.Tags[0].Key))
	assert.Equal(t, "platform", aws.ToString(u1.Tags[0].Value))

	r1, ok := roleByName["gaad-role-1"]
	require.True(t, ok)
	require.NotNil(t, r1.PermissionsBoundary)
	require.NotNil(t, r1.PermissionsBoundary.PermissionsBoundaryArn)
	assert.Equal(t, boundaryArn2, *r1.PermissionsBoundary.PermissionsBoundaryArn)
	require.Len(t, r1.Tags, 1)
	assert.Equal(t, "env", aws.ToString(r1.Tags[0].Key))
	assert.Equal(t, "prod", aws.ToString(r1.Tags[0].Value))
}

// TestGetAccountAuthorizationDetails_PolicyAttachmentFields_RealClient is a
// regression test for gopherstack-21my: ManagedPolicyDetailXML (the per-policy
// item shape for GetAccountAuthorizationDetails) omitted DefaultVersionId,
// UpdateDate, AttachmentCount, and IsAttachable entirely, even though the real
// ManagedPolicyDetail deserializer (iam@v1.58.1 deserializers.go,
// awsAwsquery_deserializeDocumentManagedPolicyDetail) reads all four, and even
// though the sibling toPolicyXML (GetPolicy/ListPolicies) already emits them
// from the same backing Policy fields. Attaches the policy to a user so
// AttachmentCount has a real, distinguishable non-zero value to assert on.
func TestGetAccountAuthorizationDetails_PolicyAttachmentFields_RealClient(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	client := newTestIAMClient(t, h)

	_, err := client.CreateUser(t.Context(), &iamsdk.CreateUserInput{UserName: aws.String("gaad-policy-user")})
	require.NoError(t, err)

	created, err := client.CreatePolicy(t.Context(), &iamsdk.CreatePolicyInput{
		PolicyName:     aws.String("gaad-attach-count-policy"),
		PolicyDocument: aws.String(testPolicyDoc),
	})
	require.NoError(t, err)

	_, err = client.AttachUserPolicy(t.Context(), &iamsdk.AttachUserPolicyInput{
		UserName:  aws.String("gaad-policy-user"),
		PolicyArn: created.Policy.Arn,
	})
	require.NoError(t, err)

	out, err := client.GetAccountAuthorizationDetails(t.Context(), &iamsdk.GetAccountAuthorizationDetailsInput{})
	require.NoError(t, err)

	var found *types.ManagedPolicyDetail
	for i := range out.Policies {
		if aws.ToString(out.Policies[i].Arn) == aws.ToString(created.Policy.Arn) {
			found = &out.Policies[i]
		}
	}

	require.NotNil(t, found, "created policy must appear in GetAccountAuthorizationDetails Policies")
	require.NotNil(t, found.DefaultVersionId)
	assert.Equal(t, "v1", *found.DefaultVersionId)
	require.NotNil(t, found.AttachmentCount)
	assert.Equal(t, int32(1), *found.AttachmentCount)
	require.NotNil(t, found.UpdateDate)
}
