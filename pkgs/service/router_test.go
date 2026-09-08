package service_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

type mockService struct {
	name       string
	target     string
	pathPrefix string
	ops        []string
	priority   int
}

func (m *mockService) Name() string {
	return m.name
}

func (m *mockService) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return c.String(http.StatusOK, "handled_by_"+m.name)
	}
}

func (m *mockService) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		req := c.Request()
		if req == nil {
			return false
		}
		if m.target != "" {
			target := req.Header.Get("X-Amz-Target")
			if len(target) >= len(m.target) && target[:len(m.target)] == m.target {
				return true
			}
		}
		if m.pathPrefix != "" && req.URL.Path == m.pathPrefix {
			return true
		}

		return false
	}
}

func (m *mockService) GetSupportedOperations() []string {
	return m.ops
}

func (m *mockService) ExtractOperation(_ *echo.Context) string {
	return "MockOp"
}

func (m *mockService) ExtractResource(_ *echo.Context) string {
	return "MockResource"
}

func (m *mockService) MatchPriority() int {
	return m.priority
}

func TestRouter_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		headerKey      string
		headerVal      string
		reqPath        string
		expectedBody   string
		expectedStatus int
	}{
		{
			name:           "fast_path_ssm_header",
			headerKey:      "X-Amz-Target",
			headerVal:      "AmazonSSM.ListDocuments",
			reqPath:        "/",
			expectedBody:   "handled_by_SSM",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "fast_path_dynamodb_header",
			headerKey:      "X-Amz-Target",
			headerVal:      "DynamoDB_20120810.GetItem",
			reqPath:        "/",
			expectedBody:   "handled_by_DynamoDB",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "fast_path_lowercase_header",
			headerKey:      "x-amz-target",
			headerVal:      "AmazonSSM.GetParameter",
			reqPath:        "/",
			expectedBody:   "handled_by_SSM",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "path_based_routing_no_target",
			headerKey:      "",
			headerVal:      "",
			reqPath:        "/s3/bucket",
			expectedBody:   "handled_by_S3",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "unmatched_fallback_to_next",
			headerKey:      "",
			headerVal:      "",
			reqPath:        "/unknown/route",
			expectedBody:   "fallback_handler",
			expectedStatus: http.StatusOK,
		},
	}

	registry := service.NewRegistry()
	require.NoError(t, registry.Register(&mockService{
		name:     "SSM",
		target:   "AmazonSSM.",
		priority: 100,
	}))
	require.NoError(t, registry.Register(&mockService{
		name:     "DynamoDB",
		target:   "DynamoDB_20120810.",
		priority: 100,
	}))
	require.NoError(t, registry.Register(&mockService{
		name:       "S3",
		pathPrefix: "/s3/bucket",
		priority:   0,
	}))

	router := service.NewServiceRouter(registry)
	fallbackHandler := func(c *echo.Context) error {
		return c.String(http.StatusOK, "fallback_handler")
	}
	handler := router.RouteHandler()(fallbackHandler)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, tt.reqPath, nil)
			if tt.headerKey != "" {
				req.Header.Set(tt.headerKey, tt.headerVal)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler(c)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, rec.Code)
			assert.Equal(t, tt.expectedBody, rec.Body.String())
		})
	}
}

type mockBenchService struct {
	mockService
}

func (m *mockBenchService) Handler() echo.HandlerFunc {
	return func(_ *echo.Context) error {
		return nil
	}
}

func BenchmarkRouter_Dispatch(b *testing.B) {
	registry := service.NewRegistry()
	for i := range 100 {
		name := "Service_" + string(rune('A'+i))
		_ = registry.Register(&mockBenchService{
			name:     name,
			target:   name + ".",
			priority: 50,
		})
	}
	_ = registry.Register(&mockBenchService{
		name:     "TargetService",
		target:   "TargetService.",
		priority: 10,
	})

	router := service.NewServiceRouter(registry)
	fallbackHandler := func(_ *echo.Context) error {
		return nil
	}
	handler := router.RouteHandler()(fallbackHandler)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Amz-Target", "TargetService.SomeOperation")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Prime cache
	_ = handler(c)

	b.ResetTimer()
	for b.Loop() {
		_ = handler(c)
	}
}
