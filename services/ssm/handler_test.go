package ssm_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ssm"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

func newTestHandler(t *testing.T) (*ssm.Handler, *ssm.InMemoryBackend) {
	t.Helper()

	backend := ssm.NewInMemoryBackend()

	return ssm.NewHandler(backend), backend
}

func doRequest(
	t *testing.T,
	h *ssm.Handler,
	action string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()

	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	} else {
		req = httptest.NewRequest(http.MethodPost, "/", nil)
	}

	if action != "" {
		req.Header.Set("X-Amz-Target", "AmazonSSM."+action)
	}

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func TestHandler_Routing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup           func(b *ssm.InMemoryBackend)
		name            string
		method          string
		target          string
		body            string
		wantBodyContain string
		wantStatus      int
	}{
		{
			name:   "GetParameter",
			method: http.MethodPost,
			target: "AmazonSSM.GetParameter",
			body:   `{"Name":"test-param"}`,
			setup: func(b *ssm.InMemoryBackend) {
				b.PutParameter(
					context.TODO(),
					&ssm.PutParameterInput{Name: "test-param", Type: "String", Value: "test-value"},
				)
			},
			wantStatus:      http.StatusOK,
			wantBodyContain: "test-value",
		},
		{
			name:            "UnknownAction",
			method:          http.MethodPost,
			target:          "AmazonSSM.FakeAction",
			body:            `{}`,
			wantStatus:      http.StatusBadRequest,
			wantBodyContain: "UnknownOperationException",
		},
		{
			name:            "MissingTarget",
			method:          http.MethodPost,
			target:          "",
			body:            `{}`,
			wantStatus:      http.StatusBadRequest,
			wantBodyContain: "Missing X-Amz-Target",
		},
		{
			name:            "GetSupportedOperations",
			method:          http.MethodGet,
			target:          "",
			body:            ``,
			wantStatus:      http.StatusOK,
			wantBodyContain: "GetParameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()

			backend := ssm.NewInMemoryBackend()
			handler := ssm.NewHandler(backend)

			if tt.setup != nil {
				tt.setup(backend)
			}

			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, "/", strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, "/", nil)
			}

			if tt.target != "" {
				req.Header.Set("X-Amz-Target", tt.target)
			}

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantBodyContain != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBodyContain)
			}
		})
	}
}

// --- Handler interface tests ---

func TestHandler_Interface(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)

	assert.Equal(t, "SSM", h.Name())
	assert.Equal(t, 100, h.MatchPriority())

	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Amz-Target", "AmazonSSM.GetParameter")
	c := e.NewContext(req, httptest.NewRecorder())
	assert.Equal(t, "GetParameter", h.ExtractOperation(c))

	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.Header.Set("X-Amz-Target", "AmazonSSMNoSep")
	c2 := e.NewContext(req2, httptest.NewRecorder())
	assert.Equal(t, "Unknown", h.ExtractOperation(c2))

	body := `{"Name":"/my/param"}`
	req3 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	c3 := e.NewContext(req3, httptest.NewRecorder())
	assert.Equal(t, "/my/param", h.ExtractResource(c3))

	req4 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	c4 := e.NewContext(req4, httptest.NewRecorder())
	assert.Empty(t, h.ExtractResource(c4))
}

// --- Provider tests ---

func TestProvider(t *testing.T) {
	t.Parallel()

	p := &ssm.Provider{}
	assert.Equal(t, "SSM", p.Name())

	ctx := &service.AppContext{Logger: slog.Default()}
	svc, err := p.Init(ctx)
	require.NoError(t, err)
	assert.NotNil(t, svc)
}

// --- Handler error cases ---

func TestHandler_ErrorCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(b *ssm.InMemoryBackend)
		name       string
		target     string
		body       string
		wantErrTyp string
		wantStatus int
	}{
		{
			name:       "ParameterNotFound",
			target:     "AmazonSSM.GetParameter",
			body:       `{"Name":"/missing/param"}`,
			wantStatus: http.StatusBadRequest,
			wantErrTyp: "ParameterNotFound",
		},
		{
			name:   "ParameterAlreadyExists",
			target: "AmazonSSM.PutParameter",
			body:   `{"Name":"/existing","Type":"String","Value":"v2","Overwrite":false}`,
			setup: func(b *ssm.InMemoryBackend) {
				b.PutParameter(
					context.TODO(),
					&ssm.PutParameterInput{Name: "/existing", Type: "String", Value: "v1"},
				)
			},
			wantStatus: http.StatusBadRequest,
			wantErrTyp: "ParameterAlreadyExists",
		},
		{
			name:       "InvalidTarget",
			target:     "AmazonSSMNoSep",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()

			backend := ssm.NewInMemoryBackend()
			h := ssm.NewHandler(backend)

			if tt.setup != nil {
				tt.setup(backend)
			}

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			req.Header.Set("X-Amz-Target", tt.target)
			rec := httptest.NewRecorder()

			require.NoError(t, h.Handler()(e.NewContext(req, rec)))
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantErrTyp != "" {
				var errResp service.JSONErrorResponse
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
				assert.Equal(t, tt.wantErrTyp, errResp.Type)
			}
		})
	}
}

// --- ParamMatchesFilter tests ---

func TestParamMatchesFilter_Options(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filters   []ssm.ParameterFilter
		wantCount int
	}{
		{
			name: "Contains",
			filters: []ssm.ParameterFilter{
				{Key: "Name", Option: "Contains", Values: []string{"db"}},
			},
			wantCount: 1,
		},
		{
			name: "UnknownKeyIgnored",
			filters: []ssm.ParameterFilter{
				{Key: "UnknownKey", Option: "Equals", Values: []string{"anything"}},
			},
			wantCount: 3,
		},
		{
			name: "DefaultOptionIsEquals",
			filters: []ssm.ParameterFilter{
				{Key: "Type", Values: []string{"SecureString"}},
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := ssm.NewInMemoryBackend()
			_, _ = backend.PutParameter(
				context.TODO(),
				&ssm.PutParameterInput{Name: "/app/db/host", Type: "String", Value: "localhost"},
			)
			_, _ = backend.PutParameter(
				context.TODO(),
				&ssm.PutParameterInput{
					Name:  "/app/cache/host",
					Type:  "SecureString",
					Value: "cache",
				},
			)
			_, _ = backend.PutParameter(
				context.TODO(),
				&ssm.PutParameterInput{Name: "/other/key", Type: "String", Value: "v"},
			)

			out, err := backend.DescribeParameters(context.TODO(), &ssm.DescribeParametersInput{
				ParameterFilters: tt.filters,
			})
			require.NoError(t, err)
			assert.Len(t, out.Parameters, tt.wantCount)
		})
	}
}

// --- ParseNextToken bad token test ---

func TestParseNextToken_BadToken(t *testing.T) {
	t.Parallel()

	backend := ssm.NewInMemoryBackend()
	for i := range 3 {
		_, _ = backend.PutParameter(context.TODO(), &ssm.PutParameterInput{
			Name: "/p" + string(rune('0'+i)), Type: "String", Value: "v",
		})
	}

	out, err := backend.DescribeParameters(context.TODO(), &ssm.DescribeParametersInput{
		NextToken: "not-a-number",
	})
	require.NoError(t, err)
	assert.Len(t, out.Parameters, 3)
}

// --- Handler HTTP via-HTTP tests ---

func TestHandler_GetParametersByPathViaHTTP(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandler(t)

	_, _ = backend.PutParameter(context.TODO(), &ssm.PutParameterInput{Name: "/svc/a", Type: "String", Value: "1"})
	_, _ = backend.PutParameter(context.TODO(), &ssm.PutParameterInput{Name: "/svc/b", Type: "String", Value: "2"})
	_, _ = backend.PutParameter(
		context.TODO(),
		&ssm.PutParameterInput{Name: "/other/c", Type: "String", Value: "3"},
	)

	rec := doRequest(t, h, "GetParametersByPath", `{"Path":"/svc","Recursive":true}`)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out ssm.GetParametersByPathOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Len(t, out.Parameters, 2)
}

func TestHandler_DescribeParametersViaHTTP(t *testing.T) {
	t.Parallel()

	h, backend := newTestHandler(t)

	_, _ = backend.PutParameter(context.TODO(), &ssm.PutParameterInput{Name: "/a", Type: "String", Value: "1"})
	_, _ = backend.PutParameter(
		context.TODO(),
		&ssm.PutParameterInput{Name: "/b", Type: "SecureString", Value: "2"},
	)

	rec := doRequest(t, h, "DescribeParameters", `{}`)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out ssm.DescribeParametersOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Len(t, out.Parameters, 2)
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	e := echo.New()

	req := httptest.NewRequest(http.MethodPut, "/", nil)
	rec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandler_ParameterOpsViaHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup           func(b *ssm.InMemoryBackend)
		name            string
		action          string
		body            string
		wantBodyContain string
		wantStatus      int
	}{
		{
			name:            "PutParameter",
			action:          "PutParameter",
			body:            `{"Name":"/http/put","Type":"String","Value":"v1"}`,
			wantStatus:      http.StatusOK,
			wantBodyContain: "Version",
		},
		{
			name:   "GetParameter",
			action: "GetParameter",
			body:   `{"Name":"/http/get"}`,
			setup: func(b *ssm.InMemoryBackend) {
				b.PutParameter(
					context.TODO(),
					&ssm.PutParameterInput{Name: "/http/get", Type: "String", Value: "val"},
				)
			},
			wantStatus:      http.StatusOK,
			wantBodyContain: "val",
		},
		{
			name:   "GetParameters",
			action: "GetParameters",
			body:   `{"Names":["/http/a","/http/b","missing"]}`,
			setup: func(b *ssm.InMemoryBackend) {
				b.PutParameter(context.TODO(), &ssm.PutParameterInput{Name: "/http/a", Type: "String", Value: "a"})
				b.PutParameter(context.TODO(), &ssm.PutParameterInput{Name: "/http/b", Type: "String", Value: "b"})
			},
			wantStatus:      http.StatusOK,
			wantBodyContain: "InvalidParameters",
		},
		{
			name:   "GetParameterHistory",
			action: "GetParameterHistory",
			body:   `{"Name":"/http/hist"}`,
			setup: func(b *ssm.InMemoryBackend) {
				b.PutParameter(
					context.TODO(),
					&ssm.PutParameterInput{Name: "/http/hist", Type: "String", Value: "v1"},
				)
				b.PutParameter(
					context.TODO(),
					&ssm.PutParameterInput{
						Name:      "/http/hist",
						Type:      "String",
						Value:     "v2",
						Overwrite: true,
					},
				)
			},
			wantStatus:      http.StatusOK,
			wantBodyContain: "v2",
		},
		{
			name:   "DeleteParameter",
			action: "DeleteParameter",
			body:   `{"Name":"/http/del"}`,
			setup: func(b *ssm.InMemoryBackend) {
				b.PutParameter(
					context.TODO(),
					&ssm.PutParameterInput{Name: "/http/del", Type: "String", Value: "v"},
				)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "DeleteParameters",
			action: "DeleteParameters",
			body:   `{"Names":["/http/d1","missing"]}`,
			setup: func(b *ssm.InMemoryBackend) {
				b.PutParameter(context.TODO(), &ssm.PutParameterInput{Name: "/http/d1", Type: "String", Value: "v"})
			},
			wantStatus:      http.StatusOK,
			wantBodyContain: "DeletedParameters",
		},
		{
			name:   "AddTagsToResource",
			action: "AddTagsToResource",
			body:   `{"ResourceType":"Parameter","ResourceId":"/http/tag","Tags":[{"Key":"k","Value":"v"}]}`,
			setup: func(b *ssm.InMemoryBackend) {
				b.PutParameter(
					context.TODO(),
					&ssm.PutParameterInput{Name: "/http/tag", Type: "String", Value: "v"},
				)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "RemoveTagsFromResource",
			action: "RemoveTagsFromResource",
			body:   `{"ResourceType":"Parameter","ResourceId":"/http/tag","TagKeys":["k"]}`,
			setup: func(b *ssm.InMemoryBackend) {
				b.PutParameter(
					context.TODO(),
					&ssm.PutParameterInput{Name: "/http/tag", Type: "String", Value: "v"},
				)
				b.AddTagsToResource(context.TODO(), &ssm.AddTagsToResourceInput{
					ResourceID: "/http/tag", Tags: []ssm.Tag{{Key: "k", Value: "v"}},
				})
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "ListTagsForResource",
			action: "ListTagsForResource",
			body:   `{"ResourceType":"Parameter","ResourceId":"/http/tag"}`,
			setup: func(b *ssm.InMemoryBackend) {
				b.PutParameter(
					context.TODO(),
					&ssm.PutParameterInput{Name: "/http/tag", Type: "String", Value: "v"},
				)
				b.AddTagsToResource(context.TODO(), &ssm.AddTagsToResourceInput{
					ResourceID: "/http/tag", Tags: []ssm.Tag{{Key: "k", Value: "v"}},
				})
			},
			wantStatus:      http.StatusOK,
			wantBodyContain: "TagList",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, backend := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(backend)
			}

			rec := doRequest(t, h, tt.action, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantBodyContain != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBodyContain)
			}
		})
	}
}

func TestHandler_RouteMatcher(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	matcher := h.RouteMatcher()
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Amz-Target", "AmazonSSM.GetParameter")
	c := e.NewContext(req, httptest.NewRecorder())
	assert.True(t, matcher(c))

	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.Header.Set("X-Amz-Target", "Other.Action")
	c2 := e.NewContext(req2, httptest.NewRecorder())
	assert.False(t, matcher(c2))
}

// --- ValidateParameterName tests ---

func TestValidateParameterName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		paramName string
		wantErr   bool
	}{
		{name: "valid path", paramName: "/my/param", wantErr: false},
		{name: "valid simple", paramName: "MyParam", wantErr: false},
		{name: "double slash", paramName: "/my//param", wantErr: true},
		{name: "reserved ssm", paramName: "ssm/something", wantErr: true},
		{name: "reserved aws", paramName: "aws-param", wantErr: true},
		{name: "reserved amazon", paramName: "amazon.param", wantErr: true},
		{name: "invalid char", paramName: "/my param!", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := ssm.NewInMemoryBackend()
			_, err := backend.PutParameter(context.TODO(), &ssm.PutParameterInput{
				Name:  tc.paramName,
				Type:  "String",
				Value: "val",
			})
			if tc.wantErr {
				require.Error(t, err)
				// ParameterPatternMismatchException, not the generic
				// ValidationException: it is PutParameter's own declared
				// exception for a malformed Name (gopherstack-jpfk).
				assert.ErrorIs(t, err, ssm.ErrParameterNamePattern)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
