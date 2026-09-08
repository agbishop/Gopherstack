package iam_test

import (
	"encoding/xml"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	iamsdk "github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/sdkcheck"
	"github.com/blackbirdworks/gopherstack/services/iam"
)

// errSimulated is a sentinel error used to exercise InternalFailure handling.
var errSimulated = errors.New("simulated internal error")

// errBackend wraps InMemoryBackend and overrides CreateUser to return a raw (non-sentinel) error.
// This is used to exercise the InternalFailure code path in handleError.
type errBackend struct {
	*iam.InMemoryBackend
}

func (e *errBackend) CreateUser(_, _, _ string) (*iam.User, error) {
	return nil, errSimulated
}

// iamRequest creates a form-encoded IAM HTTP request.

func iamRequest(action string, params map[string]string) *http.Request {
	vals := url.Values{}
	vals.Set("Action", action)
	vals.Set("Version", "2010-05-08")
	for k, v := range params {
		vals.Set(k, v)
	}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return req
}

func newTestHandler(t *testing.T) (*iam.Handler, *iam.InMemoryBackend) {
	t.Helper()
	b := iam.NewInMemoryBackend()
	h := iam.NewHandler(b)

	return h, b
}

func TestIAMHandler_Routing(t *testing.T) {
	t.Parallel()

	t.Run("MissingActionReturns400", func(t *testing.T) {
		t.Parallel()
		e := echo.New()
		h, _ := newTestHandler(t)

		req := httptest.NewRequest(http.MethodPost, "/",
			strings.NewReader("Version=2010-05-08"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := h.Handler()(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("UnknownActionReturns400", func(t *testing.T) {
		t.Parallel()
		e := echo.New()
		h, _ := newTestHandler(t)

		req := iamRequest("NonExistentAction", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := h.Handler()(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var errResp iam.ErrorResponse
		require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
		assert.Equal(t, "InvalidAction", errResp.Error.Code)
	})

	t.Run("GetMethodReturnsOperationList", func(t *testing.T) {
		t.Parallel()
		e := echo.New()
		h, _ := newTestHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := h.Handler()(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "CreateUser")
	})

	t.Run("WrongMethodReturns405", func(t *testing.T) {
		t.Parallel()
		e := echo.New()
		h, _ := newTestHandler(t)

		req := httptest.NewRequest(http.MethodPut, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := h.Handler()(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("EntityAlreadyExistsError", func(t *testing.T) {
		t.Parallel()
		e := echo.New()
		h, b := newTestHandler(t)
		_, _ = b.CreateUser("alice", "/", "")

		req := iamRequest("CreateUser", map[string]string{"UserName": "alice"})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := h.Handler()(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusConflict, rec.Code)

		var errResp iam.ErrorResponse
		require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
		assert.Equal(t, "EntityAlreadyExists", errResp.Error.Code)
	})

	t.Run("RouteMatcher_MatchesIAMRequests", func(t *testing.T) {
		t.Parallel()
		e := echo.New()
		h, _ := newTestHandler(t)

		matcher := h.RouteMatcher()

		req := iamRequest("CreateUser", map[string]string{"UserName": "alice"})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		assert.True(t, matcher(c))
	})

	t.Run("RouteMatcher_RejectsNonIAM", func(t *testing.T) {
		t.Parallel()
		e := echo.New()
		h, _ := newTestHandler(t)

		matcher := h.RouteMatcher()

		// JSON request (not form-encoded)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"Action":"ListUsers"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		assert.False(t, matcher(c))
	})

	t.Run("RouteMatcher_RejectsGETRequests", func(t *testing.T) {
		t.Parallel()
		e := echo.New()
		h, _ := newTestHandler(t)

		matcher := h.RouteMatcher()

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		assert.False(t, matcher(c))
	})

	t.Run("ExtractOperation", func(t *testing.T) {
		t.Parallel()
		e := echo.New()
		h, _ := newTestHandler(t)

		req := iamRequest("CreateUser", map[string]string{"UserName": "alice"})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		assert.Equal(t, "CreateUser", h.ExtractOperation(c))
	})

	t.Run("ExtractResource", func(t *testing.T) {
		t.Parallel()
		e := echo.New()
		h, _ := newTestHandler(t)

		req := iamRequest("CreateUser", map[string]string{"UserName": "alice"})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		assert.Equal(t, "alice", h.ExtractResource(c))
	})

	t.Run("MatchPriority", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t)
		assert.Equal(t, 80, h.MatchPriority())
	})

	t.Run("Name", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t)
		assert.Equal(t, "IAM", h.Name())
	})

	t.Run("GetSupportedOperations", func(t *testing.T) {
		t.Parallel()
		h, _ := newTestHandler(t)
		ops := h.GetSupportedOperations()
		assert.Contains(t, ops, "CreateUser")
		assert.Contains(t, ops, "CreateRole")
		assert.Contains(t, ops, "CreatePolicy")
	})
}

// TestIAMHandler_ExtractEdgeCases covers remaining branches in ExtractOperation,
// ExtractResource, RouteMatcher, and Handler().

func TestIAMHandler_ExtractEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("ExtractOperation_NoAction", func(t *testing.T) {
		t.Parallel()
		e := echo.New()
		h, _ := newTestHandler(t)

		// Body has no Action param → returns "Unknown"
		req := httptest.NewRequest(http.MethodPost, "/",
			strings.NewReader("Version=2010-05-08"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		assert.Equal(t, "Unknown", h.ExtractOperation(c))
	})

	t.Run("ExtractResource_NoMatchingKey", func(t *testing.T) {
		t.Parallel()
		e := echo.New()
		h, _ := newTestHandler(t)

		// ListUsers has no UserName/RoleName/etc → returns ""
		req := iamRequest("ListUsers", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		assert.Empty(t, h.ExtractResource(c))
	})

	t.Run("RouteMatcher_FormEncodedMissingVersion", func(t *testing.T) {
		t.Parallel()
		e := echo.New()
		h, _ := newTestHandler(t)
		matcher := h.RouteMatcher()

		// Form-encoded POST but body doesn't contain the IAM version string
		req := httptest.NewRequest(http.MethodPost, "/",
			strings.NewReader("Action=ListUsers"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		assert.False(t, matcher(c))
	})

	t.Run("Handler_GETNonRoot_Returns405", func(t *testing.T) {
		t.Parallel()
		e := echo.New()
		h, _ := newTestHandler(t)

		// GET at a non-root path falls through to the method-not-allowed branch
		req := httptest.NewRequest(http.MethodGet, "/some/path", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := h.Handler()(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})
}

// TestListSorting_MultipleItems adds tests with 2+ items to exercise
// the comparison closures inside [sort.Slice] calls.

func TestListSorting_MultipleItems(t *testing.T) {
	t.Parallel()

	t.Run("ListPolicies_TwoItems", func(t *testing.T) {
		t.Parallel()
		b := iam.NewInMemoryBackend()
		_, _ = b.CreatePolicy("ZPolicy", "/", "")
		_, _ = b.CreatePolicy("APolicy", "/", "")
		policies, err := b.ListPolicies("", 0)
		require.NoError(t, err)
		require.Len(t, policies.Data, 2)
		assert.Equal(t, "APolicy", policies.Data[0].PolicyName)
	})

	t.Run("ListAccessKeys_TwoKeys", func(t *testing.T) {
		t.Parallel()
		b := iam.NewInMemoryBackend()
		_, _ = b.CreateUser("alice", "/", "")
		_, _ = b.CreateAccessKey("alice")
		_, _ = b.CreateAccessKey("alice")
		keys, err := b.ListAccessKeys("alice", "", 0)
		require.NoError(t, err)
		require.Len(t, keys.Data, 2)
	})

	t.Run("ListAllAccessKeys_TwoKeys", func(t *testing.T) {
		t.Parallel()
		b := iam.NewInMemoryBackend()
		_, _ = b.CreateUser("alice", "/", "")
		_, _ = b.CreateAccessKey("alice")
		_, _ = b.CreateAccessKey("alice")
		keys := b.ListAllAccessKeys()
		require.Len(t, keys, 2)
	})

	t.Run("ListInstanceProfiles_TwoItems", func(t *testing.T) {
		t.Parallel()
		b := iam.NewInMemoryBackend()
		_, _ = b.CreateInstanceProfile("ZProfile", "/")
		_, _ = b.CreateInstanceProfile("AProfile", "/")
		profiles, err := b.ListInstanceProfiles("", 0)
		require.NoError(t, err)
		require.Len(t, profiles.Data, 2)
		assert.Equal(t, "AProfile", profiles.Data[0].InstanceProfileName)
	})
}

// TestIAMHandler_ServiceFailure tests the ServiceFailure error code path
// by injecting a backend that returns a raw (non-sentinel) error.
//
// Real AWS IAM (query protocol) reports unhandled server errors as
// "ServiceFailure", not the JSON-protocol-style "InternalFailure" — see
// https://docs.aws.amazon.com/IAM/latest/APIReference/API_CreateUser.html#API_CreateUser_Errors.

func TestIAMHandler_ServiceFailure(t *testing.T) {
	t.Parallel()

	e := echo.New()
	eb := &errBackend{iam.NewInMemoryBackend()}
	h := iam.NewHandler(eb)

	req := iamRequest("CreateUser", map[string]string{"UserName": "alice"})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp iam.ErrorResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "ServiceFailure", errResp.Error.Code)
}

// TestIAMHandler_DispatchErrors covers the error branches in every dispatch case.
// These tests trigger "NoSuchEntity" / "EntityAlreadyExists" responses from the backend.
//
// wantStatus values match the HTTP status codes documented per-operation by the
// real AWS IAM API reference (e.g. NoSuchEntity=404, EntityAlreadyExists=409,
// DeleteConflict=409, LimitExceeded=409); MalformedPolicyDocument stays 400.
// See e.g. https://docs.aws.amazon.com/IAM/latest/APIReference/API_CreateUser.html#API_CreateUser_Errors
// and https://docs.aws.amazon.com/IAM/latest/APIReference/API_DeleteUser.html#API_DeleteUser_Errors.

func TestIAMHandler_DispatchErrors(t *testing.T) {
	t.Parallel()

	// helper to assert the correct error code in the XML response
	assertErrorCode := func(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
		t.Helper()
		assert.Equal(t, wantStatus, rec.Code)

		var errResp iam.ErrorResponse
		require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
		assert.Equal(t, wantCode, errResp.Error.Code)
	}

	for _, tc := range []struct {
		name       string
		action     string
		params     map[string]string
		setup      func(*iam.InMemoryBackend)
		wantCode   string
		wantStatus int
	}{
		{
			name:       "DeleteUser_NotFound",
			action:     "DeleteUser",
			params:     map[string]string{"UserName": "nobody"},
			wantCode:   "NoSuchEntity",
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "CreateRole_AlreadyExists",
			action: "CreateRole",
			params: map[string]string{"RoleName": "MyRole"},
			setup: func(b *iam.InMemoryBackend) {
				_, _ = b.CreateRole("MyRole", "/", "", "")
			},
			wantCode:   "EntityAlreadyExists",
			wantStatus: http.StatusConflict,
		},
		{
			name:       "GetRole_NotFound",
			action:     "GetRole",
			params:     map[string]string{"RoleName": "ghost"},
			wantCode:   "NoSuchEntity",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "DeleteRole_NotFound",
			action:     "DeleteRole",
			params:     map[string]string{"RoleName": "ghost"},
			setup:      func(_ *iam.InMemoryBackend) {},
			wantCode:   "NoSuchEntity",
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "CreatePolicy_AlreadyExists",
			action: "CreatePolicy",
			params: map[string]string{"PolicyName": "MyPolicy"},
			setup: func(b *iam.InMemoryBackend) {
				_, _ = b.CreatePolicy("MyPolicy", "/", "")
			},
			wantCode:   "EntityAlreadyExists",
			wantStatus: http.StatusConflict,
		},
		{
			name:       "DeletePolicy_NotFound",
			action:     "DeletePolicy",
			params:     map[string]string{"PolicyArn": "arn:aws:iam::000000000000:policy/ghost"},
			wantCode:   "NoSuchEntity",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "AttachUserPolicy_UserNotFound",
			action:     "AttachUserPolicy",
			params:     map[string]string{"UserName": "nobody", "PolicyArn": "arn:aws:iam::000000000000:policy/P"},
			wantCode:   "NoSuchEntity",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "AttachRolePolicy_RoleNotFound",
			action:     "AttachRolePolicy",
			params:     map[string]string{"RoleName": "ghost", "PolicyArn": "arn:aws:iam::000000000000:policy/P"},
			wantCode:   "NoSuchEntity",
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "CreateGroup_AlreadyExists",
			action: "CreateGroup",
			params: map[string]string{"GroupName": "Admins"},
			setup: func(b *iam.InMemoryBackend) {
				_, _ = b.CreateGroup("Admins", "/")
			},
			wantCode:   "EntityAlreadyExists",
			wantStatus: http.StatusConflict,
		},
		{
			name:       "DeleteGroup_NotFound",
			action:     "DeleteGroup",
			params:     map[string]string{"GroupName": "ghost"},
			wantCode:   "NoSuchEntity",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "AddUserToGroup_GroupNotFound",
			action:     "AddUserToGroup",
			params:     map[string]string{"GroupName": "ghost", "UserName": "alice"},
			wantCode:   "NoSuchEntity",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "CreateAccessKey_UserNotFound",
			action:     "CreateAccessKey",
			params:     map[string]string{"UserName": "nobody"},
			wantCode:   "NoSuchEntity",
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "DeleteAccessKey_NotFound",
			action: "DeleteAccessKey",
			params: map[string]string{"UserName": "alice", "AccessKeyId": "AKIANONEXISTENT"},
			setup: func(b *iam.InMemoryBackend) {
				_, _ = b.CreateUser("alice", "/", "")
			},
			wantCode:   "NoSuchEntity",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "ListAccessKeys_UserNotFound",
			action:     "ListAccessKeys",
			params:     map[string]string{"UserName": "nobody"},
			wantCode:   "NoSuchEntity",
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "CreateInstanceProfile_AlreadyExists",
			action: "CreateInstanceProfile",
			params: map[string]string{"InstanceProfileName": "MyProfile"},
			setup: func(b *iam.InMemoryBackend) {
				_, _ = b.CreateInstanceProfile("MyProfile", "/")
			},
			wantCode:   "EntityAlreadyExists",
			wantStatus: http.StatusConflict,
		},
		{
			name:       "DeleteInstanceProfile_NotFound",
			action:     "DeleteInstanceProfile",
			params:     map[string]string{"InstanceProfileName": "ghost"},
			wantCode:   "NoSuchEntity",
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "DeleteUser_DeleteConflict",
			action: "DeleteUser",
			params: map[string]string{"UserName": "alice"},
			setup: func(b *iam.InMemoryBackend) {
				_, _ = b.CreateUser("alice", "/", "")
				pol, _ := b.CreatePolicy("StuckPolicy", "/", "")
				_ = b.AttachUserPolicy("alice", pol.Arn)
			},
			wantCode:   "DeleteConflict",
			wantStatus: http.StatusConflict,
		},
		{
			name:   "DeleteRole_DeleteConflict",
			action: "DeleteRole",
			params: map[string]string{"RoleName": "MyRole"},
			setup: func(b *iam.InMemoryBackend) {
				_, _ = b.CreateRole("MyRole", "/", "", "")
				pol, _ := b.CreatePolicy("RolePolicy", "/", "")
				_ = b.AttachRolePolicy("MyRole", pol.Arn)
			},
			wantCode:   "DeleteConflict",
			wantStatus: http.StatusConflict,
		},
		{
			name:   "CreateRole_MalformedPolicyDocument",
			action: "CreateRole",
			params: map[string]string{
				"RoleName":                 "BadRole",
				"AssumeRolePolicyDocument": "not-json",
			},
			wantCode:   "MalformedPolicyDocument",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "CreatePolicy_MalformedPolicyDocument",
			action: "CreatePolicy",
			params: map[string]string{
				"PolicyName":     "BadPolicy",
				"PolicyDocument": "not-json",
			},
			wantCode:   "MalformedPolicyDocument",
			wantStatus: http.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := echo.New()
			h, b := newTestHandler(t)
			if tc.setup != nil {
				tc.setup(b)
			}

			req := iamRequest(tc.action, tc.params)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := h.Handler()(c)
			require.NoError(t, err)
			assertErrorCode(t, rec, tc.wantStatus, tc.wantCode)
		})
	}
}

// TestSDKCompleteness verifies that every operation exposed by the AWS SDK v2
// iam client is either listed in GetSupportedOperations() or explicitly
// acknowledged in the notImplemented slice.  The test fails when the upstream
// SDK adds a new operation that gopherstack has not yet handled.

func TestSDKCompleteness(t *testing.T) {
	t.Parallel()

	backend := iam.NewInMemoryBackend()
	h := iam.NewHandler(backend)

	// Added by the aws-sdk-go-v2/service/iam v1.63.0 bump; unimplemented.
	notImplemented := []string{
		"AcquireRole",
		"GetAccountProperties",
		"GetRoleTemplateVersion",
		"PutAccountProperties",
	}
	sdkcheck.CheckCompleteness(t, &iamsdk.Client{}, h.GetSupportedOperations(), notImplemented)
}

// callIAM is a helper that sends an IAM action to the handler and returns the recorder.

func callIAM(t *testing.T, h *iam.Handler, action string, params map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	req := iamRequest(action, params)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h.Handler()(c))

	return rec
}

func timeNow() time.Time {
	return time.Now()
}

func iamCall(
	t *testing.T, e *echo.Echo, h *iam.Handler, action string, params map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := iamRequest(action, params)
	rec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req, rec)))

	return rec
}

// extractFirstXMLTag extracts the text of the first occurrence of <tag>text</tag>.

func extractFirstXMLTag(body, tag string) string {
	openTag := "<" + tag + ">"
	closeTag := "</" + tag + ">"
	idx := strings.Index(body, openTag)
	if idx < 0 {
		return ""
	}
	start := idx + len(openTag)
	end := strings.Index(body[start:], closeTag)
	if end < 0 {
		return ""
	}

	return body[start : start+end]
}

// Test_ErrorHTTPStatusCodes verifies each IAM error sentinel maps to the
// exact HTTP status code AWS documents per-operation (e.g. NoSuchEntity=404,
// EntityAlreadyExists=409), not a blanket 400. See e.g.
// https://docs.aws.amazon.com/IAM/latest/APIReference/API_CreateUser.html#API_CreateUser_Errors
// and https://docs.aws.amazon.com/IAM/latest/APIReference/API_DeleteUser.html#API_DeleteUser_Errors.

func mergeParams(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	maps.Copy(out, base)

	maps.Copy(out, extra)

	return out
}

func TestRouteMatcher_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup func() *http.Request
		name  string
		want  bool
	}{
		{
			name: "GET_request_returns_false",
			setup: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

				return r
			},
			want: false,
		},
		{
			name: "dashboard_path_returns_false",
			setup: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/dashboard/iam", nil)
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

				return r
			},
			want: false,
		},
		{
			name: "non_form_content_type_returns_false",
			setup: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/",
					strings.NewReader(`{"Action":"ListUsers"}`))
				r.Header.Set("Content-Type", "application/json")

				return r
			},
			want: false,
		},
		{
			name: "valid_iam_request_returns_true",
			setup: func() *http.Request {
				vals := url.Values{}
				vals.Set("Action", "ListUsers")
				vals.Set("Version", "2010-05-08")
				r := httptest.NewRequest(http.MethodPost, "/",
					strings.NewReader(vals.Encode()))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

				return r
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)
			e := echo.New()
			matcher := h.RouteMatcher()
			req := tt.setup()
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			assert.Equal(t, tt.want, matcher(c))
		})
	}
}

func TestExtractOperation_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupReq func() *http.Request
		name     string
		want     string
	}{
		{
			name: "valid_action_extracted",
			setupReq: func() *http.Request {
				return iamRequest("CreateUser", nil)
			},
			want: "CreateUser",
		},
		{
			name: "no_action_returns_unknown",
			setupReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/",
					strings.NewReader("Version=2010-05-08"))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

				return r
			},
			want: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)
			e := echo.New()
			req := tt.setupReq()
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			assert.Equal(t, tt.want, h.ExtractOperation(c))
		})
	}
}

func TestExtractResource_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupReq func() *http.Request
		name     string
		want     string
	}{
		{
			name: "username_extracted",
			setupReq: func() *http.Request {
				return iamRequest("CreateUser", map[string]string{"UserName": "alice"})
			},
			want: "alice",
		},
		{
			name: "rolename_extracted",
			setupReq: func() *http.Request {
				return iamRequest("CreateRole", map[string]string{"RoleName": "MyRole"})
			},
			want: "MyRole",
		},
		{
			name: "policyname_extracted",
			setupReq: func() *http.Request {
				return iamRequest("CreatePolicy", map[string]string{"PolicyName": "MyPolicy"})
			},
			want: "MyPolicy",
		},
		{
			name: "groupname_extracted",
			setupReq: func() *http.Request {
				return iamRequest("CreateGroup", map[string]string{"GroupName": "Admins"})
			},
			want: "Admins",
		},
		{
			name: "instance_profile_name_extracted",
			setupReq: func() *http.Request {
				return iamRequest("CreateInstanceProfile",
					map[string]string{"InstanceProfileName": "MyProfile"})
			},
			want: "MyProfile",
		},
		{
			name: "no_resource_key_returns_empty",
			setupReq: func() *http.Request {
				return iamRequest("ListUsers", nil)
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)
			e := echo.New()
			req := tt.setupReq()
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			assert.Equal(t, tt.want, h.ExtractResource(c))
		})
	}
}

func TestHandler_EntryPoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupReq    func() *http.Request
		name        string
		wantContain string
		wantCode    int
	}{
		{
			name: "GET_root_returns_supported_operations",
			setupReq: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/", nil)
			},
			wantCode:    http.StatusOK,
			wantContain: "CreateUser",
		},
		{
			name: "non_POST_non_root_GET_returns_405",
			setupReq: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/some/path", nil)
			},
			wantCode:    http.StatusMethodNotAllowed,
			wantContain: "Method not allowed",
		},
		{
			name: "PUT_returns_405",
			setupReq: func() *http.Request {
				return httptest.NewRequest(http.MethodPut, "/", nil)
			},
			wantCode:    http.StatusMethodNotAllowed,
			wantContain: "Method not allowed",
		},
		{
			name: "POST_with_invalid_body_returns_400",
			setupReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/",
					strings.NewReader("%ZZ")) // invalid URL encoding
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

				return r
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "POST_missing_action_returns_400",
			setupReq: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/",
					strings.NewReader("Version=2010-05-08"))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

				return r
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)
			e := echo.New()
			req := tt.setupReq()
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := h.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCode, rec.Code)
			if tt.wantContain != "" {
				assert.Contains(t, rec.Body.String(), tt.wantContain)
			}
		})
	}
}

func TestMarshalXML_ErrorPath(t *testing.T) {
	t.Parallel()

	// Create a handler and attempt to write an error that triggers the XML marshal path
	// by sending an unrecognized action - the handler will call writeError which uses marshalXML
	h, _ := newTestHandler(t)
	e := echo.New()

	req := iamRequest("UnknownAction", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)
	// Should get a 400 for unknown action with proper XML
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidAction")
}

func TestIAMHandler_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	ops := h.GetSupportedOperations()

	assert.Contains(t, ops, "CreateUser")
	assert.Contains(t, ops, "CreateRole")
	assert.Contains(t, ops, "AttachRolePolicy")
	assert.NotEmpty(t, ops)
}

// iamRequestIndexed builds a request with indexed member parameters.
// params is a flat map; caller must encode member indices in keys.

func iamRequestWithMembers(action string, params map[string]string, indexed map[string][]string) *http.Request {
	base := maps.Clone(params)
	if base == nil {
		base = map[string]string{}
	}

	for prefix, values := range indexed {
		for i, v := range values {
			base[fmt.Sprintf("%s.%d", prefix, i+1)] = v
		}
	}

	return iamRequest(action, base)
}
