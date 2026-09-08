package lambda_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/services/lambda"
)

// ---- mock backend ----

type mockBackend struct {
	invokeErr    error
	functions    map[string]*lambda.FunctionConfiguration
	invokeResult []byte
	invokeCount  int
	mu           sync.RWMutex
}

func newMockBackend() *mockBackend {
	return &mockBackend{functions: make(map[string]*lambda.FunctionConfiguration)}
}

func (m *mockBackend) CreateFunction(fn *lambda.FunctionConfiguration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.functions[fn.FunctionName]; exists {
		return lambda.ErrFunctionAlreadyExists
	}

	m.functions[fn.FunctionName] = fn

	return nil
}

func (m *mockBackend) GetFunction(name string) (*lambda.FunctionConfiguration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	fn, ok := m.functions[name]
	if !ok {
		return nil, lambda.ErrFunctionNotFound
	}

	return fn, nil
}

func (m *mockBackend) ListFunctions(marker string, maxItems int) page.Page[*lambda.FunctionConfiguration] {
	m.mu.RLock()
	defer m.mu.RUnlock()

	fns := make([]*lambda.FunctionConfiguration, 0, len(m.functions))
	for _, fn := range m.functions {
		fns = append(fns, fn)
	}

	sort.Slice(fns, func(i, j int) bool {
		return fns[i].FunctionName < fns[j].FunctionName
	})

	return page.New(fns, marker, maxItems, 50)
}

func (m *mockBackend) DeleteFunction(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.functions[name]; !ok {
		return lambda.ErrFunctionNotFound
	}

	delete(m.functions, name)

	return nil
}

func (m *mockBackend) UpdateFunction(fn *lambda.FunctionConfiguration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.functions[fn.FunctionName]; !ok {
		return lambda.ErrFunctionNotFound
	}

	m.functions[fn.FunctionName] = fn

	return nil
}

func (m *mockBackend) InvokeFunction(
	_ context.Context,
	name string,
	invocationType lambda.InvocationType,
	_ []byte,
) ([]byte, int, error) {
	m.invokeCount++

	if m.invokeErr != nil {
		return nil, http.StatusInternalServerError, m.invokeErr
	}

	if invocationType == lambda.InvocationTypeDryRun {
		return nil, http.StatusNoContent, nil
	}

	if invocationType == lambda.InvocationTypeEvent {
		return nil, http.StatusAccepted, nil
	}

	if _, ok := m.functions[name]; !ok {
		return nil, http.StatusNotFound, lambda.ErrFunctionNotFound
	}

	result := m.invokeResult
	if result == nil {
		result = []byte(`{"result":"ok"}`)
	}

	return result, http.StatusOK, nil
}

func (m *mockBackend) Purge(_ context.Context, _ time.Time) {}

// ---- helpers ----

func newHandler(t *testing.T) (*lambda.Handler, *mockBackend) {
	t.Helper()

	bk := newMockBackend()
	h := lambda.NewHandler(bk)
	h.DefaultRegion = "us-east-1"
	h.AccountID = "000000000000"

	return h, bk
}

func callHandler(
	t *testing.T,
	h *lambda.Handler,
	method, path, body string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("")
	}

	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func TestCreateFunction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup                func(*mockBackend)
		name                 string
		body                 string
		wantErrType          string
		wantFunctionName     string
		wantFunctionArn      string
		wantPackageType      string
		wantState            lambda.FunctionState
		wantLastUpdateStatus lambda.LastUpdateStatus
		wantCode             int
		wantMemorySize       int
		wantTimeout          int
		wantRevisionID       bool
	}{
		{
			name: "success",
			body: `{"FunctionName":"my-func","PackageType":"Image",` +
				`"Code":{"ImageUri":"123456789012.dkr.ecr.us-east-1.amazonaws.com/myimage:latest"},` +
				`"Role":"arn:aws:iam::000000000000:role/myrole"}`,
			wantCode:             http.StatusCreated,
			wantFunctionName:     "my-func",
			wantFunctionArn:      "arn:aws:lambda:us-east-1:000000000000:function:my-func",
			wantPackageType:      lambda.PackageTypeImage,
			wantState:            lambda.FunctionStateActive,
			wantLastUpdateStatus: lambda.LastUpdateStatusSuccessful,
			wantMemorySize:       128,
			wantTimeout:          3,
			wantRevisionID:       true,
		},
		{
			name: "defaults_applied",
			body: `{"FunctionName":"defaults-func","PackageType":"Image",` +
				`"Code":{"ImageUri":"myimage:latest"},"MemorySize":256,"Timeout":60}`,
			wantCode:             http.StatusCreated,
			wantFunctionName:     "defaults-func",
			wantLastUpdateStatus: lambda.LastUpdateStatusSuccessful,
			wantMemorySize:       256,
			wantTimeout:          60,
		},
		{
			name:        "missing_function_name",
			body:        `{"PackageType":"Image","Code":{"ImageUri":"myimage:latest"}}`,
			wantCode:    http.StatusBadRequest,
			wantErrType: "InvalidParameterValueException",
		},
		{
			name:        "invalid_package_type",
			body:        `{"FunctionName":"zip-func","PackageType":"Zip","Code":{"S3Bucket":"mybucket","S3Key":"code.zip"}}`,
			wantCode:    http.StatusBadRequest,
			wantErrType: "InvalidParameterValueException",
		},
		{
			name:        "missing_image_uri",
			body:        `{"FunctionName":"no-image-func","PackageType":"Image","Code":{}}`,
			wantCode:    http.StatusBadRequest,
			wantErrType: "InvalidParameterValueException",
		},
		{
			name:     "invalid_body",
			body:     "not-json{",
			wantCode: http.StatusBadRequest,
		},
		{
			name: "already_exists",
			setup: func(bk *mockBackend) {
				_ = bk.CreateFunction(&lambda.FunctionConfiguration{
					FunctionName: "dup-func",
					PackageType:  lambda.PackageTypeImage,
					ImageURI:     "myimage:latest",
				})
			},
			body:        `{"FunctionName":"dup-func","PackageType":"Image","Code":{"ImageUri":"myimage:latest"}}`,
			wantCode:    http.StatusConflict,
			wantErrType: "ResourceConflictException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, bk := newHandler(t)
			if tt.setup != nil {
				tt.setup(bk)
			}

			rec := callHandler(t, h, http.MethodPost, "/2015-03-31/functions", tt.body, nil)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantErrType != "" {
				assertLambdaError(t, rec, tt.wantErrType)
			}

			if tt.wantFunctionName == "" {
				return
			}

			var fn lambda.FunctionConfiguration
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &fn))
			assert.Equal(t, tt.wantFunctionName, fn.FunctionName)
			if tt.wantFunctionArn != "" {
				assert.Equal(t, tt.wantFunctionArn, fn.FunctionArn)
			}
			if tt.wantPackageType != "" {
				assert.Equal(t, tt.wantPackageType, fn.PackageType)
			}
			if tt.wantState != "" {
				assert.Equal(t, tt.wantState, fn.State)
			}
			if tt.wantLastUpdateStatus != "" {
				assert.Equal(t, tt.wantLastUpdateStatus, fn.LastUpdateStatus)
			}
			if tt.wantMemorySize > 0 {
				assert.Equal(t, tt.wantMemorySize, fn.MemorySize)
			}
			if tt.wantTimeout > 0 {
				assert.Equal(t, tt.wantTimeout, fn.Timeout)
			}
			if tt.wantRevisionID {
				assert.NotEmpty(t, fn.RevisionID)
			}
		})
	}
}

func TestGetFunction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*mockBackend)
		name         string
		funcName     string
		wantErrType  string
		wantImageURI string
		wantRepoType string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(bk *mockBackend) {
				bk.functions["get-func"] = &lambda.FunctionConfiguration{
					FunctionName: "get-func",
					FunctionArn:  "arn:aws:lambda:us-east-1:000000000000:function:get-func",
					ImageURI:     "myimage:latest",
					PackageType:  lambda.PackageTypeImage,
					State:        lambda.FunctionStateActive,
				}
			},
			funcName:     "get-func",
			wantCode:     http.StatusOK,
			wantImageURI: "myimage:latest",
			wantRepoType: "ECR",
		},
		{
			name:        "not_found",
			funcName:    "nonexistent",
			wantCode:    http.StatusNotFound,
			wantErrType: "ResourceNotFoundException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, bk := newHandler(t)
			if tt.setup != nil {
				tt.setup(bk)
			}

			rec := callHandler(t, h, http.MethodGet, "/2015-03-31/functions/"+tt.funcName, "", nil)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantErrType != "" {
				assertLambdaError(t, rec, tt.wantErrType)
			}

			if tt.wantCode == http.StatusOK {
				var out lambda.GetFunctionOutput
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				require.NotNil(t, out.Configuration)
				assert.Equal(t, tt.funcName, out.Configuration.FunctionName)
				require.NotNil(t, out.Code)
				assert.Equal(t, tt.wantImageURI, out.Code.ImageURI)
				assert.Equal(t, tt.wantRepoType, out.Code.RepositoryType)
			}
		})
	}
}

func TestListFunctions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(*mockBackend)
		name      string
		wantCount int
	}{
		{
			name:      "empty",
			wantCount: 0,
		},
		{
			name: "multiple",
			setup: func(bk *mockBackend) {
				bk.functions["func-a"] = &lambda.FunctionConfiguration{FunctionName: "func-a"}
				bk.functions["func-b"] = &lambda.FunctionConfiguration{FunctionName: "func-b"}
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, bk := newHandler(t)
			if tt.setup != nil {
				tt.setup(bk)
			}

			rec := callHandler(t, h, http.MethodGet, "/2015-03-31/functions", "", nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var out lambda.ListFunctionsOutput
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Len(t, out.Functions, tt.wantCount)
		})
	}
}

func TestDeleteFunction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup       func(*mockBackend)
		name        string
		funcName    string
		wantErrType string
		wantCode    int
		wantEmpty   bool
	}{
		{
			name: "success",
			setup: func(bk *mockBackend) {
				bk.functions["del-func"] = &lambda.FunctionConfiguration{FunctionName: "del-func"}
			},
			funcName:  "del-func",
			wantCode:  http.StatusNoContent,
			wantEmpty: true,
		},
		{
			name:        "not_found",
			funcName:    "missing",
			wantCode:    http.StatusNotFound,
			wantErrType: "ResourceNotFoundException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, bk := newHandler(t)
			if tt.setup != nil {
				tt.setup(bk)
			}

			rec := callHandler(t, h, http.MethodDelete, "/2015-03-31/functions/"+tt.funcName, "", nil)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantErrType != "" {
				assertLambdaError(t, rec, tt.wantErrType)
			}

			if tt.wantEmpty {
				assert.Empty(t, bk.functions)
			}
		})
	}
}

func TestUpdateFunctionCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup                func(*mockBackend)
		name                 string
		funcName             string
		body                 string
		wantImageURI         string
		wantLastUpdateStatus lambda.LastUpdateStatus
		wantCode             int
	}{
		{
			name: "success",
			setup: func(bk *mockBackend) {
				bk.functions["code-func"] = &lambda.FunctionConfiguration{
					FunctionName: "code-func",
					ImageURI:     "old-image:v1",
				}
			},
			funcName:             "code-func",
			body:                 `{"ImageUri":"new-image:v2"}`,
			wantCode:             http.StatusOK,
			wantImageURI:         "new-image:v2",
			wantLastUpdateStatus: lambda.LastUpdateStatusSuccessful,
		},
		{
			name:     "not_found",
			funcName: "missing",
			body:     `{"ImageUri":"new-image:v2"}`,
			wantCode: http.StatusNotFound,
		},
		{
			name: "missing_image_uri",
			setup: func(bk *mockBackend) {
				bk.functions["code-func"] = &lambda.FunctionConfiguration{FunctionName: "code-func"}
			},
			funcName: "code-func",
			body:     `{}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name: "invalid_body",
			setup: func(bk *mockBackend) {
				bk.functions["code-func"] = &lambda.FunctionConfiguration{FunctionName: "code-func"}
			},
			funcName: "code-func",
			body:     "bad{json}",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, bk := newHandler(t)
			if tt.setup != nil {
				tt.setup(bk)
			}

			rec := callHandler(t, h, http.MethodPut, "/2015-03-31/functions/"+tt.funcName+"/code", tt.body, nil)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantImageURI != "" {
				var fn lambda.FunctionConfiguration
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &fn))
				assert.Equal(t, tt.wantImageURI, fn.ImageURI)
				assert.NotEmpty(t, fn.RevisionID)
				if tt.wantLastUpdateStatus != "" {
					assert.Equal(t, tt.wantLastUpdateStatus, fn.LastUpdateStatus)
				}
			}
		})
	}
}

func TestUpdateFunctionConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup                func(*mockBackend)
		name                 string
		funcName             string
		body                 string
		wantDescription      string
		wantRole             string
		wantEnvKey           string
		wantEnvValue         string
		wantLastUpdateStatus lambda.LastUpdateStatus
		wantCode             int
		wantMemorySize       int
		wantTimeout          int
	}{
		{
			name: "success",
			setup: func(bk *mockBackend) {
				bk.functions["cfg-func"] = &lambda.FunctionConfiguration{
					FunctionName: "cfg-func",
					MemorySize:   128,
					Timeout:      3,
					Description:  "old description",
				}
			},
			funcName:             "cfg-func",
			body:                 `{"Description":"new description","MemorySize":512,"Timeout":30,"Role":"new-role"}`,
			wantCode:             http.StatusOK,
			wantDescription:      "new description",
			wantMemorySize:       512,
			wantTimeout:          30,
			wantRole:             "new-role",
			wantLastUpdateStatus: lambda.LastUpdateStatusSuccessful,
		},
		{
			name: "update_environment",
			setup: func(bk *mockBackend) {
				bk.functions["env-func"] = &lambda.FunctionConfiguration{FunctionName: "env-func"}
			},
			funcName:             "env-func",
			body:                 `{"Environment":{"Variables":{"KEY":"VALUE"}}}`,
			wantCode:             http.StatusOK,
			wantEnvKey:           "KEY",
			wantEnvValue:         "VALUE",
			wantLastUpdateStatus: lambda.LastUpdateStatusSuccessful,
		},
		{
			name:     "not_found",
			funcName: "missing",
			body:     `{}`,
			wantCode: http.StatusNotFound,
		},
		{
			name: "invalid_body",
			setup: func(bk *mockBackend) {
				bk.functions["cfg-func"] = &lambda.FunctionConfiguration{FunctionName: "cfg-func"}
			},
			funcName: "cfg-func",
			body:     "bad{json}",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, bk := newHandler(t)
			if tt.setup != nil {
				tt.setup(bk)
			}

			rec := callHandler(
				t,
				h,
				http.MethodPut,
				"/2015-03-31/functions/"+tt.funcName+"/configuration",
				tt.body,
				nil,
			)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode != http.StatusOK {
				return
			}

			var fn lambda.FunctionConfiguration
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &fn))
			if tt.wantDescription != "" {
				assert.Equal(t, tt.wantDescription, fn.Description)
			}
			if tt.wantMemorySize > 0 {
				assert.Equal(t, tt.wantMemorySize, fn.MemorySize)
			}
			if tt.wantTimeout > 0 {
				assert.Equal(t, tt.wantTimeout, fn.Timeout)
			}
			if tt.wantRole != "" {
				assert.Equal(t, tt.wantRole, fn.Role)
			}
			if tt.wantEnvKey != "" {
				require.NotNil(t, fn.Environment)
				assert.Equal(t, tt.wantEnvValue, fn.Environment.Variables[tt.wantEnvKey])
			}
			if tt.wantLastUpdateStatus != "" {
				assert.Equal(t, tt.wantLastUpdateStatus, fn.LastUpdateStatus)
			}
		})
	}
}

func TestInvoke(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*mockBackend)
		headers      map[string]string
		name         string
		funcName     string
		body         string
		wantErrType  string
		wantContains string
		wantCode     int
		wantNoInvoke bool
	}{
		{
			name: "request_response",
			setup: func(bk *mockBackend) {
				bk.functions["invoke-func"] = &lambda.FunctionConfiguration{FunctionName: "invoke-func"}
				bk.invokeResult = []byte(`{"answer":42}`)
			},
			funcName:     "invoke-func",
			body:         `{"key":"value"}`,
			wantCode:     http.StatusOK,
			wantContains: "42",
		},
		{
			name: "event",
			setup: func(bk *mockBackend) {
				bk.functions["event-func"] = &lambda.FunctionConfiguration{FunctionName: "event-func"}
			},
			funcName: "event-func",
			body:     `{}`,
			headers:  map[string]string{"X-Amz-Invocation-Type": "Event"},
			wantCode: http.StatusAccepted,
		},
		{
			name: "dry_run",
			setup: func(bk *mockBackend) {
				bk.functions["dryrun-func"] = &lambda.FunctionConfiguration{FunctionName: "dryrun-func"}
			},
			funcName: "dryrun-func",
			body:     `{}`,
			headers:  map[string]string{"X-Amz-Invocation-Type": "DryRun"},
			wantCode: http.StatusNoContent,
		},
		{
			name:        "not_found",
			funcName:    "missing",
			body:        `{}`,
			wantCode:    http.StatusNotFound,
			wantErrType: "ResourceNotFoundException",
		},
		{
			name: "service_error",
			setup: func(bk *mockBackend) {
				bk.functions["err-func"] = &lambda.FunctionConfiguration{FunctionName: "err-func"}
				bk.invokeErr = fmt.Errorf("%w: Docker unavailable", lambda.ErrLambdaUnavailable)
			},
			funcName: "err-func",
			body:     `{}`,
			wantCode: http.StatusInternalServerError,
		},
		{
			name: "empty_body",
			setup: func(bk *mockBackend) {
				bk.functions["body-func"] = &lambda.FunctionConfiguration{FunctionName: "body-func"}
			},
			funcName: "body-func",
			body:     "",
			wantCode: http.StatusOK,
		},
		{
			// validateInvocationHeaders (handler_invocation.go) used to write
			// its rejection via h.writeError and return that call's
			// (always-nil) result, so handleInvoke's `if valErr != nil` never
			// fired and the function was invoked anyway on top of the
			// already-written 400 (gopherstack-3t96, the gopherstack-8haq
			// shape). wantNoInvoke asserts the backend was never called, not
			// just the status code -- a status-only assertion passes against
			// this bug, since httptest.ResponseRecorder keeps the first
			// WriteHeader call.
			name: "invalid_invocation_type_not_invoked",
			setup: func(bk *mockBackend) {
				bk.functions["bad-invtype-func"] = &lambda.FunctionConfiguration{FunctionName: "bad-invtype-func"}
			},
			funcName:     "bad-invtype-func",
			body:         `{}`,
			headers:      map[string]string{"X-Amz-Invocation-Type": "Bogus"},
			wantCode:     http.StatusBadRequest,
			wantErrType:  "InvalidParameterValueException",
			wantNoInvoke: true,
		},
		{
			name: "invalid_log_type_not_invoked",
			setup: func(bk *mockBackend) {
				bk.functions["bad-logtype-func"] = &lambda.FunctionConfiguration{FunctionName: "bad-logtype-func"}
			},
			funcName:     "bad-logtype-func",
			body:         `{}`,
			headers:      map[string]string{"X-Amz-Log-Type": "Bogus"},
			wantCode:     http.StatusBadRequest,
			wantErrType:  "InvalidParameterValueException",
			wantNoInvoke: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, bk := newHandler(t)
			if tt.setup != nil {
				tt.setup(bk)
			}

			rec := callHandler(
				t,
				h,
				http.MethodPost,
				"/2015-03-31/functions/"+tt.funcName+"/invocations",
				tt.body,
				tt.headers,
			)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantNoInvoke {
				assert.Equal(t, 0, bk.invokeCount, "a rejected invocation header must not reach the backend")
			}

			if tt.wantErrType != "" {
				assertLambdaError(t, rec, tt.wantErrType)
			}

			if tt.wantContains != "" {
				assert.Contains(t, rec.Body.String(), tt.wantContains)
			}
		})
	}
}
