package apigateway_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/apigateway"
)

func newAPIGWHandler() *apigateway.Handler {
	return apigateway.NewHandler(apigateway.NewInMemoryBackend())
}

// createParityAPI creates a REST API and returns its ID.
func createParityAPI(t *testing.T, h *apigateway.Handler, name string) string {
	t.Helper()

	rec := restRequest(t, h, http.MethodPost, "/restapis", fmt.Sprintf(`{"name":%q}`, name))
	require.Equal(t, http.StatusCreated, rec.Code, "create api: %s", rec.Body.String())

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	id, _ := out["id"].(string)
	require.NotEmpty(t, id)

	return id
}

// getRootResourceID fetches the root resource ID of a REST API.
func getRootResourceID(t *testing.T, h *apigateway.Handler, apiID string) string {
	t.Helper()

	rec := restRequest(t, h, http.MethodGet, fmt.Sprintf("/restapis/%s/resources", apiID), "")
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		Items []map[string]any `json:"item"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	for _, r := range out.Items {
		if r["path"] == "/" {
			id, _ := r["id"].(string)

			return id
		}
	}

	t.Fatal("root resource not found")

	return ""
}

// restCall sends a raw REST-style request to h and returns the recorder,
// mirroring the shape a real aws-sdk-go-v2 apigateway client request takes
// (no X-Amz-Target header).
func restCall(
	t *testing.T, h *apigateway.Handler, method, path, contentType, body string,
) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()

	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	return rec
}

// mockAPIGWConfigProvider implements config.Provider for testing.
type mockAPIGWConfigProvider struct{}

func (m *mockAPIGWConfigProvider) GetGlobalConfig() *config.GlobalConfig {
	return config.NewGlobalConfig("111111111111", "eu-west-1", 0, 0, false, 0)
}

func TestProvider_APIGateway(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ctx         *service.AppContext
		name        string
		wantSvcName string
		wantName    bool
	}{
		{
			name:     "name_returns_APIGateway",
			wantName: true,
		},
		{
			name: "init_with_config",
			ctx: &service.AppContext{
				Logger: slog.Default(),
				Config: &mockAPIGWConfigProvider{},
			},
			wantSvcName: "APIGateway",
		},
		{
			name: "init_without_config",
			ctx:  &service.AppContext{Logger: slog.Default()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &apigateway.Provider{}

			if tt.wantName {
				assert.Equal(t, "APIGateway", p.Name())

				return
			}

			svc, err := p.Init(tt.ctx)
			require.NoError(t, err)
			require.NotNil(t, svc)

			if tt.wantSvcName != "" {
				assert.Equal(t, tt.wantSvcName, svc.Name())
			}
		})
	}
}

func TestHandler_APIGateway_Metadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		wantName      string
		wantOps       []string
		wantPriority  int
		checkPriority bool
	}{
		{
			name:     "name",
			wantName: "APIGateway",
		},
		{
			name:          "match_priority",
			wantPriority:  100,
			checkPriority: true,
		},
		{
			name:    "supported_operations",
			wantOps: []string{"CreateRestApi", "GetRestApis", "PutMethod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := apigateway.NewHandler(apigateway.NewInMemoryBackend())

			switch {
			case tt.wantName != "":
				assert.Equal(t, tt.wantName, h.Name())
			case tt.checkPriority:
				assert.Equal(t, tt.wantPriority, h.MatchPriority())
			case len(tt.wantOps) > 0:
				ops := h.GetSupportedOperations()
				for _, op := range tt.wantOps {
					assert.Contains(t, ops, op)
				}
			}
		})
	}
}

func TestHandler_APIGateway_RouteMatcher(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		target    string
		wantMatch bool
	}{
		{
			name:      "matches_APIGateway_target",
			target:    "APIGateway.CreateRestApi",
			wantMatch: true,
		},
		{
			name:      "no_match_for_other_service",
			target:    "AmazonSQS.CreateQueue",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := apigateway.NewHandler(apigateway.NewInMemoryBackend())
			matcher := h.RouteMatcher()
			e := echo.New()

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Amz-Target", tt.target)
			ctx := e.NewContext(req, httptest.NewRecorder())

			assert.Equal(t, tt.wantMatch, matcher(ctx))
		})
	}
}

func TestHandler_APIGateway_ExtractOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		wantOp string
	}{
		{
			name:   "known_target_extracts_operation",
			target: "APIGateway.CreateRestApi",
			wantOp: "CreateRestApi",
		},
		{
			name:   "missing_target_returns_Unknown",
			target: "",
			wantOp: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := apigateway.NewHandler(apigateway.NewInMemoryBackend())
			e := echo.New()

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			if tt.target != "" {
				req.Header.Set("X-Amz-Target", tt.target)
			}

			assert.Equal(t, tt.wantOp, h.ExtractOperation(e.NewContext(req, httptest.NewRecorder())))
		})
	}
}

func TestHandler_APIGateway_ExtractResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantResource string
	}{
		{
			name:         "body_with_restApiId",
			body:         `{"restApiId":"abc123"}`,
			wantResource: "abc123",
		},
		{
			name:         "body_without_restApiId",
			body:         `{}`,
			wantResource: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := apigateway.NewHandler(apigateway.NewInMemoryBackend())
			e := echo.New()

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			got := h.ExtractResource(e.NewContext(req, httptest.NewRecorder()))

			assert.Equal(t, tt.wantResource, got)
		})
	}
}

func TestHandler_APIGateway_RequestErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		method   string
		path     string
		target   string
		body     string
		wantCode int
	}{
		{
			name:     "missing_target_returns_400",
			method:   http.MethodPost,
			path:     "/",
			body:     "{}",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "PUT_method_returns_405",
			method:   http.MethodPut,
			path:     "/something",
			target:   "CreateRestApi",
			wantCode: http.StatusMethodNotAllowed,
		},
		{
			name:     "invalid_JSON_returns_500",
			method:   http.MethodPost,
			path:     "/",
			target:   "CreateRestApi",
			body:     "not-json",
			wantCode: http.StatusInternalServerError,
		},
		{
			name:     "missing_required_params_returns_400",
			method:   http.MethodPost,
			path:     "/",
			target:   "CreateRestApi",
			body:     "{}",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()

			h := apigateway.NewHandler(apigateway.NewInMemoryBackend())

			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			if tt.target != "" {
				req.Header.Set("X-Amz-Target", "APIGateway."+tt.target)
			}

			rec := httptest.NewRecorder()
			err := h.Handler()(e.NewContext(req, rec))
			require.NoError(t, err)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestHandler_APIGateway_NotFoundErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action string
		body   string
	}{
		{
			name:   "DeleteRestApi_nonexistent",
			action: "DeleteRestApi",
			body:   `{"restApiId":"nonexistent"}`,
		},
		{
			name:   "GetRestApi_nonexistent",
			action: "GetRestApi",
			body:   `{"restApiId":"nonexistent"}`,
		},
		{
			name:   "GetResources_nonexistent",
			action: "GetResources",
			body:   `{"restApiId":"nonexistent"}`,
		},
		{
			name:   "GetResource_nonexistent",
			action: "GetResource",
			body:   `{"restApiId":"nonexistent","resourceId":"r1"}`,
		},
		{
			name:   "CreateResource_nonexistent_api",
			action: "CreateResource",
			body:   `{"restApiId":"nonexistent","parentId":"r1","pathPart":"pets"}`,
		},
		{
			name:   "DeleteResource_nonexistent",
			action: "DeleteResource",
			body:   `{"restApiId":"nonexistent","resourceId":"r1"}`,
		},
		{
			name:   "PutMethod_nonexistent",
			action: "PutMethod",
			body:   `{"restApiId":"nonexistent","resourceId":"r1","httpMethod":"GET","authorizationType":"NONE"}`,
		},
		{
			name:   "GetMethod_nonexistent",
			action: "GetMethod",
			body:   `{"restApiId":"nonexistent","resourceId":"r1","httpMethod":"GET"}`,
		},
		{
			name:   "DeleteMethod_nonexistent",
			action: "DeleteMethod",
			body:   `{"restApiId":"nonexistent","resourceId":"r1","httpMethod":"GET"}`,
		},
		{
			name:   "PutIntegration_nonexistent",
			action: "PutIntegration",
			body:   `{"restApiId":"nonexistent","resourceId":"r1","httpMethod":"GET","type":"MOCK"}`,
		},
		{
			name:   "GetIntegration_nonexistent",
			action: "GetIntegration",
			body:   `{"restApiId":"nonexistent","resourceId":"r1","httpMethod":"GET"}`,
		},
		{
			name:   "DeleteIntegration_nonexistent",
			action: "DeleteIntegration",
			body:   `{"restApiId":"nonexistent","resourceId":"r1","httpMethod":"GET"}`,
		},
		{
			name:   "CreateDeployment_nonexistent",
			action: "CreateDeployment",
			body:   `{"restApiId":"nonexistent","stageName":"prod"}`,
		},
		{
			name:   "GetDeployments_nonexistent",
			action: "GetDeployments",
			body:   `{"restApiId":"nonexistent"}`,
		},
		{
			name:   "GetStages_nonexistent",
			action: "GetStages",
			body:   `{"restApiId":"nonexistent"}`,
		},
		{
			name:   "GetStage_nonexistent",
			action: "GetStage",
			body:   `{"restApiId":"nonexistent","stageName":"prod"}`,
		},
		{
			name:   "DeleteStage_nonexistent",
			action: "DeleteStage",
			body:   `{"restApiId":"nonexistent","stageName":"prod"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := post(t, tt.action, tt.body)
			assert.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}

func TestHandler_APIGateway_FullWorkflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		apiName     string
		apiDesc     string
		pathPart    string
		stageName   string
		wantCreated int
	}{
		{
			name:        "complete_REST_API_lifecycle",
			apiName:     "my-api",
			apiDesc:     "Test API",
			pathPart:    "pets",
			stageName:   "prod",
			wantCreated: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, e := sharedSetup()

			createRec := postWithHandler(t, h, e, "CreateRestApi",
				`{"name":"`+tt.apiName+`","description":"`+tt.apiDesc+`"}`)
			require.Equal(t, tt.wantCreated, createRec.Code)

			var createResp map[string]any
			require.NoError(t, parseJSON(createRec, &createResp))
			apiID := createResp["id"].(string)
			require.NotEmpty(t, apiID)

			listRec := postWithHandler(t, h, e, "GetRestApis", `{"limit":10,"position":""}`)
			assert.Equal(t, http.StatusOK, listRec.Code)

			var resourcesResp map[string]any
			resListRec := postWithHandler(t, h, e, "GetResources",
				`{"restApiId":"`+apiID+`","limit":10}`)
			require.Equal(t, http.StatusOK, resListRec.Code)
			require.NoError(t, parseJSON(resListRec, &resourcesResp))

			items := resourcesResp["item"].([]any)
			require.Len(t, items, 1)
			rootID := items[0].(map[string]any)["id"].(string)

			childRec := postWithHandler(t, h, e, "CreateResource",
				`{"restApiId":"`+apiID+`","parentId":"`+rootID+`","pathPart":"`+tt.pathPart+`"}`)
			require.Equal(t, http.StatusCreated, childRec.Code)

			var childResp map[string]any
			require.NoError(t, parseJSON(childRec, &childResp))
			resourceID := childResp["id"].(string)

			methodRec := postWithHandler(t, h, e, "PutMethod",
				`{"restApiId":"`+apiID+`","resourceId":"`+resourceID+`","httpMethod":"GET","authorizationType":"NONE"}`)
			assert.Equal(t, http.StatusCreated, methodRec.Code)

			intRec := postWithHandler(t, h, e, "PutIntegration",
				`{"restApiId":"`+apiID+`","resourceId":"`+resourceID+`","httpMethod":"GET","type":"MOCK"}`)
			assert.Equal(t, http.StatusCreated, intRec.Code)

			deplRec := postWithHandler(t, h, e, "CreateDeployment",
				`{"restApiId":"`+apiID+`","stageName":"`+tt.stageName+`","description":"Initial"}`)
			assert.Equal(t, http.StatusCreated, deplRec.Code)

			deplListRec := postWithHandler(t, h, e, "GetDeployments",
				`{"restApiId":"`+apiID+`"}`)
			assert.Equal(t, http.StatusOK, deplListRec.Code)

			stageRec := postWithHandler(t, h, e, "GetStage",
				`{"restApiId":"`+apiID+`","stageName":"`+tt.stageName+`"}`)
			assert.Equal(t, http.StatusOK, stageRec.Code)

			delStageRec := postWithHandler(t, h, e, "DeleteStage",
				`{"restApiId":"`+apiID+`","stageName":"`+tt.stageName+`"}`)
			assert.Equal(t, http.StatusNoContent, delStageRec.Code)

			delMethodRec := postWithHandler(t, h, e, "DeleteMethod",
				`{"restApiId":"`+apiID+`","resourceId":"`+resourceID+`","httpMethod":"GET"}`)
			assert.Equal(t, http.StatusNoContent, delMethodRec.Code)

			delResRec := postWithHandler(t, h, e, "DeleteResource",
				`{"restApiId":"`+apiID+`","resourceId":"`+resourceID+`"}`)
			assert.Equal(t, http.StatusNoContent, delResRec.Code)

			delRec := postWithHandler(t, h, e, "DeleteRestApi", `{"restApiId":"`+apiID+`"}`)
			assert.Equal(t, http.StatusAccepted, delRec.Code)
		})
	}
}

// parseJSON is a helper to decode JSON from a response recorder.
func parseJSON(rec *httptest.ResponseRecorder, v any) error {
	return json.Unmarshal(rec.Body.Bytes(), v)
}

// errNoopNotImplemented is returned by noopBackend for methods that are not expected
// to be called in the fallback-persistence tests.
var errNoopNotImplemented = errors.New("not implemented")

// noopBackend implements StorageBackend without Snapshot/Restore so we can test
// the persistence fallback branches in Handler.Snapshot and Handler.Restore.
type noopBackend struct{}

func (n *noopBackend) CreateRestAPI(_ apigateway.CreateRestAPIInput) (*apigateway.RestAPI, error) {
	return &apigateway.RestAPI{ID: "x", Name: "x"}, nil
}

func (n *noopBackend) DeleteRestAPI(_ string) error { return nil }

func (n *noopBackend) GetRestAPI(_ string) (*apigateway.RestAPI, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetRestAPIs(_ int, _ string) ([]apigateway.RestAPI, string, error) {
	return nil, "", nil
}

func (n *noopBackend) GetResources(_ string, _ string, _ int) ([]apigateway.Resource, string, error) {
	return nil, "", nil
}

func (n *noopBackend) ResourcesForRouting(_ string) ([]apigateway.Resource, uint64, error) {
	return nil, 0, nil
}

func (n *noopBackend) GetResource(_ string, _ string) (*apigateway.Resource, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) CreateResource(_ string, _ string, _ string) (*apigateway.Resource, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteResource(_ string, _ string) error { return nil }

func (n *noopBackend) PutMethod(_ apigateway.PutMethodInput) (*apigateway.Method, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetMethod(_ string, _ string, _ string) (*apigateway.Method, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteMethod(_ string, _ string, _ string) error { return nil }

func (n *noopBackend) PutMethodResponse(
	_ string, _ string, _ string, _ string, _ apigateway.PutMethodResponseInput,
) (*apigateway.MethodResponse, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetMethodResponse(_ string, _ string, _ string, _ string) (*apigateway.MethodResponse, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteMethodResponse(_ string, _ string, _ string, _ string) error { return nil }

func (n *noopBackend) PutIntegration(
	_ string, _ string, _ string, _ apigateway.PutIntegrationInput,
) (*apigateway.Integration, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetIntegration(_ string, _ string, _ string) (*apigateway.Integration, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteIntegration(_ string, _ string, _ string) error { return nil }

func (n *noopBackend) PutIntegrationResponse(
	_ string, _ string, _ string, _ string, _ apigateway.PutIntegrationResponseInput,
) (*apigateway.IntegrationResponse, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetIntegrationResponse(
	_ string,
	_ string,
	_ string,
	_ string,
) (*apigateway.IntegrationResponse, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteIntegrationResponse(_ string, _ string, _ string, _ string) error {
	return nil
}

func (n *noopBackend) CreateDeployment(_ string, _ string, _ string) (*apigateway.Deployment, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetDeployment(_ string, _ string) (*apigateway.Deployment, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetDeployments(_ string) ([]apigateway.Deployment, error) {
	return nil, nil
}

func (n *noopBackend) DeleteDeployment(_ string, _ string) error { return nil }

func (n *noopBackend) GetStages(_ string) ([]apigateway.Stage, error) { return nil, nil }

func (n *noopBackend) GetStage(_ string, _ string) (*apigateway.Stage, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteStage(_ string, _ string) error { return nil }

func (n *noopBackend) CreateAuthorizer(_ string, _ apigateway.CreateAuthorizerInput) (*apigateway.Authorizer, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetAuthorizer(_ string, _ string) (*apigateway.Authorizer, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetAuthorizers(_ string) ([]apigateway.Authorizer, error) { return nil, nil }

func (n *noopBackend) UpdateAuthorizer(
	_ string,
	_ string,
	_ apigateway.UpdateAuthorizerInput,
) (*apigateway.Authorizer, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteAuthorizer(_ string, _ string) error { return nil }

func (n *noopBackend) CreateRequestValidator(
	_ string,
	_ apigateway.CreateRequestValidatorInput,
) (*apigateway.RequestValidator, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetRequestValidator(_ string, _ string) (*apigateway.RequestValidator, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetRequestValidators(_ string) ([]apigateway.RequestValidator, error) {
	return nil, nil
}

func (n *noopBackend) UpdateRequestValidator(
	_ string,
	_ string,
	_ apigateway.UpdateRequestValidatorInput,
) (*apigateway.RequestValidator, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteRequestValidator(_ string, _ string) error { return nil }

func (n *noopBackend) CreateAPIKey(_ apigateway.CreateAPIKeyInput) (*apigateway.APIKey, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) CreateBasePathMapping(
	_ apigateway.CreateBasePathMappingInput,
) (*apigateway.BasePathMapping, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) CreateDocumentationPart(
	_ apigateway.CreateDocumentationPartInput,
) (*apigateway.DocumentationPart, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) CreateDocumentationVersion(
	_ apigateway.CreateDocumentationVersionInput,
) (*apigateway.DocumentationVersion, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) CreateDomainName(_ apigateway.CreateDomainNameInput) (*apigateway.DomainName, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) CreateDomainNameAccessAssociation(
	_ apigateway.CreateDomainNameAccessAssociationInput,
) (*apigateway.DomainNameAccessAssociation, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) CreateModel(_ apigateway.CreateModelInput) (*apigateway.Model, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) CreateStage(_ apigateway.CreateStageInput) (*apigateway.Stage, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) CreateUsagePlan(_ apigateway.CreateUsagePlanInput) (*apigateway.UsagePlan, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) CreateUsagePlanKey(_ apigateway.CreateUsagePlanKeyInput) (*apigateway.UsagePlanKey, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetAPIKey(_ string) (*apigateway.APIKey, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetAPIKeyByValue(_ string) (*apigateway.APIKey, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetAPIKeys() ([]apigateway.APIKey, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteAPIKey(_ string) error { return errNoopNotImplemented }

func (n *noopBackend) UpdateAPIKey(_ string, _ apigateway.UpdateAPIKeyInput) (*apigateway.APIKey, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetDomainName(_ string) (*apigateway.DomainName, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetDomainNames(_ string) ([]apigateway.DomainName, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteDomainName(_ string) error { return errNoopNotImplemented }

func (n *noopBackend) GetBasePathMapping(_ string, _ string) (*apigateway.BasePathMapping, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetBasePathMappings(_ string) ([]apigateway.BasePathMapping, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteBasePathMapping(_ string, _ string) error { return errNoopNotImplemented }

func (n *noopBackend) GetModel(_ string, _ string) (*apigateway.Model, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetModels(_ string) ([]apigateway.Model, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteModel(_ string, _ string) error { return errNoopNotImplemented }

func (n *noopBackend) UpdateModel(_ string, _ string, _ apigateway.UpdateModelInput) (*apigateway.Model, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) UpdateStage(_ string, _ string, _ apigateway.UpdateStageInput) (*apigateway.Stage, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetUsagePlan(_ string) (*apigateway.UsagePlan, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetUsagePlans() ([]apigateway.UsagePlan, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetUsagePlansForKey(_ string) ([]apigateway.UsagePlan, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteUsagePlan(_ string) error { return errNoopNotImplemented }

func (n *noopBackend) GetUsagePlanKey(_ string, _ string) (*apigateway.UsagePlanKey, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetUsagePlanKeys(_ string) ([]apigateway.UsagePlanKey, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteUsagePlanKey(_ string, _ string) error { return errNoopNotImplemented }

func (n *noopBackend) GetDocumentationPart(_ string, _ string) (*apigateway.DocumentationPart, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetDocumentationParts(_ string) ([]apigateway.DocumentationPart, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteDocumentationPart(_ string, _ string) error {
	return errNoopNotImplemented
}

func (n *noopBackend) GetDocumentationVersion(_ string, _ string) (*apigateway.DocumentationVersion, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetDocumentationVersions(_ string) ([]apigateway.DocumentationVersion, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteDocumentationVersion(_ string, _ string) error {
	return errNoopNotImplemented
}

func (n *noopBackend) UpdateRestAPI(_ string, _ apigateway.UpdateRestAPIInput) (*apigateway.RestAPI, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) UpdateResource(
	_ string,
	_ string,
	_ apigateway.UpdateResourceInput,
) (*apigateway.Resource, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) UpdateDeployment(
	_ string,
	_ string,
	_ apigateway.UpdateDeploymentInput,
) (*apigateway.Deployment, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetAccount() (*apigateway.Account, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetResourceTags(_ string) (map[string]string, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) TagResource(_ string, _ map[string]string) error {
	return errNoopNotImplemented
}

func (n *noopBackend) UntagResource(_ string, _ []string) error {
	return errNoopNotImplemented
}

func (n *noopBackend) TestInvokeMethod(_ apigateway.TestInvokeMethodInput) (*apigateway.TestInvokeMethodOutput, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetAPIKeysPage(_ int, _ string) ([]apigateway.APIKey, string, error) {
	return nil, "", errNoopNotImplemented
}

func (n *noopBackend) GetDomainNamesPage(_ int, _ string) ([]apigateway.DomainName, string, error) {
	return nil, "", errNoopNotImplemented
}

func (n *noopBackend) GetUsagePlansPage(_ int, _ string) ([]apigateway.UsagePlan, string, error) {
	return nil, "", errNoopNotImplemented
}

func (n *noopBackend) UpdateUsagePlan(_ apigateway.UpdateUsagePlanInput) (*apigateway.UsagePlan, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) UpdateDomainName(_ apigateway.UpdateDomainNameInput) (*apigateway.DomainName, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) UpdateBasePathMapping(
	_ apigateway.UpdateBasePathMappingInput,
) (*apigateway.BasePathMapping, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) UpdateDocumentationPart(
	_ apigateway.UpdateDocumentationPartInput,
) (*apigateway.DocumentationPart, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) UpdateDocumentationVersion(
	_ apigateway.UpdateDocumentationVersionInput,
) (*apigateway.DocumentationVersion, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) UpdateMethod(_ apigateway.UpdateMethodInput) (*apigateway.Method, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) UpdateIntegration(_ apigateway.UpdateIntegrationInput) (*apigateway.Integration, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) UpdateIntegrationResponse(
	_ apigateway.UpdateIntegrationResponseInput,
) (*apigateway.IntegrationResponse, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) UpdateMethodResponse(_ apigateway.UpdateMethodResponseInput) (*apigateway.MethodResponse, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) UpdateAccount(_ apigateway.UpdateAccountInput) (*apigateway.Account, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) TestInvokeAuthorizer(
	_ apigateway.TestInvokeAuthorizerInput,
) (*apigateway.TestInvokeAuthorizerOutput, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetModelTemplate(_ string, _ string) (string, error) {
	return "", errNoopNotImplemented
}

func (n *noopBackend) GetGatewayResponse(_ string, _ string) (*apigateway.GatewayResponse, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetGatewayResponses(_ string) ([]apigateway.GatewayResponse, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) PutGatewayResponse(_ apigateway.PutGatewayResponseInput) (*apigateway.GatewayResponse, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) UpdateGatewayResponse(_ apigateway.PutGatewayResponseInput) (*apigateway.GatewayResponse, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteGatewayResponse(_ string, _ string) error { return errNoopNotImplemented }

func (n *noopBackend) GenerateClientCertificate(
	_ apigateway.GenerateClientCertificateInput,
) (*apigateway.ClientCertificate, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetClientCertificate(_ string) (*apigateway.ClientCertificate, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetClientCertificates() ([]apigateway.ClientCertificate, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteClientCertificate(_ string) error { return errNoopNotImplemented }

func (n *noopBackend) GetUsage(_ apigateway.GetUsageInput) (*apigateway.UsageData, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) EnforceUsagePlan(_, _, _ string) error {
	return nil
}

func (n *noopBackend) EnforceMethodThrottle(_, _, _, _ string) error {
	return nil
}

func (n *noopBackend) CreateVpcLink(_ apigateway.CreateVpcLinkInput) (*apigateway.VpcLink, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetVpcLink(_ string) (*apigateway.VpcLink, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetVpcLinks() ([]apigateway.VpcLink, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteVpcLink(_ string) error { return errNoopNotImplemented }

func (n *noopBackend) UpdateVpcLink(_ apigateway.UpdateVpcLinkInput) (*apigateway.VpcLink, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) UpdateClientCertificate(
	_ apigateway.UpdateClientCertificateInput,
) (*apigateway.ClientCertificate, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetExport(_ string, _ string, _ string) (map[string]any, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetDomainNameAccessAssociations(
	_ string,
) ([]apigateway.DomainNameAccessAssociation, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) DeleteDomainNameAccessAssociation(_ string) error {
	return errNoopNotImplemented
}

func (n *noopBackend) RejectDomainNameAccessAssociation(_, _ string) error {
	return errNoopNotImplemented
}

func (n *noopBackend) GetSdkTypes() []apigateway.SdkType { return nil }

func (n *noopBackend) GetSdkType(_ string) (*apigateway.SdkType, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) GetSdk(_, _, _ string) (*apigateway.SdkExport, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) ImportAPIKeys(_ []byte, _ string, _ bool) ([]string, []string, error) {
	return nil, nil, errNoopNotImplemented
}

func (n *noopBackend) ImportDocumentationParts(
	_ string, _ []byte, _ string, _ bool,
) ([]string, []string, error) {
	return nil, nil, errNoopNotImplemented
}

func (n *noopBackend) UpdateUsage(_, _ string, _ map[string]string) (*apigateway.UsageData, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) ImportRestAPI(_ apigateway.ImportRestAPIInput) (*apigateway.RestAPI, error) {
	return nil, errNoopNotImplemented
}

func (n *noopBackend) PutRestAPI(_ apigateway.PutRestAPIInput) (*apigateway.RestAPI, error) {
	return nil, errNoopNotImplemented
}

// restRequest sends a REST-style request (no X-Amz-Target header) to the handler.
func restRequest(t *testing.T, handler *apigateway.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := handler.Handler()(c)
	require.NoError(t, err)

	return rec
}

// TestHandlerPersistence_NoopBackend covers the fallback branches in Handler.Snapshot
// and Handler.Restore when the backend does not implement those interfaces.
func TestHandlerPersistence_NoopBackend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wantNilSnap bool
		wantNoErr   bool
	}{
		{
			name:        "Snapshot_returns_nil_for_non_snapshotter",
			wantNilSnap: true,
		},
		{
			name:      "Restore_returns_nil_for_non_restorer",
			wantNoErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := apigateway.NewHandler(&noopBackend{})

			if tt.wantNilSnap {
				snap := h.Snapshot(t.Context())
				assert.Nil(t, snap)

				return
			}

			err := h.Restore(t.Context(), []byte(`{"apis":{}}`))
			require.NoError(t, err)
		})
	}
}

// TestHandleRESTAPI_Branches covers the branches inside handleRESTAPI that are not
// hit by the existing REST-path test: unknown path → 404, dispatch error → 4xx,
// and successful DELETE that returns 204.
func TestHandleRESTAPI_Branches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(b *apigateway.InMemoryBackend) string
		name     string
		method   string
		path     string
		body     string
		wantCode int
	}{
		{
			// gopherstack-wlo1: handleRESTAPI's dispatch-miss fallback now
			// routes through handleError(errUnknownOperation), a typed
			// UnknownOperationException at 400 -- not the bare "not found"
			// text/plain 404 this test previously encoded.
			name:     "unknown_rest_path_returns_400",
			method:   http.MethodGet,
			path:     "/restapis/abc/unknownsegment",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "dispatch_error_nonexistent_api",
			method:   http.MethodGet,
			path:     "/restapis/nonexistent",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "delete_resource_returns_204",
			method:   http.MethodDelete,
			wantCode: http.StatusNoContent,
			setup: func(b *apigateway.InMemoryBackend) string {
				api, _ := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "api"})
				resources, _, _ := b.GetResources(api.ID, "", 0)
				rootID := resources[0].ID
				child, _ := b.CreateResource(api.ID, rootID, "items")

				return fmt.Sprintf("/restapis/%s/resources/%s", api.ID, child.ID)
			},
		},
		{
			name:     "delete_stage_returns_204",
			method:   http.MethodDelete,
			wantCode: http.StatusNoContent,
			setup: func(b *apigateway.InMemoryBackend) string {
				api, _ := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "api"})
				_, _ = b.CreateDeployment(api.ID, "prod", "")

				return fmt.Sprintf("/restapis/%s/stages/prod", api.ID)
			},
		},
		{
			name:     "delete_method_returns_204",
			method:   http.MethodDelete,
			wantCode: http.StatusNoContent,
			setup: func(b *apigateway.InMemoryBackend) string {
				api, _ := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "api"})
				resources, _, _ := b.GetResources(api.ID, "", 0)
				rootID := resources[0].ID
				_, _ = b.PutMethod(
					apigateway.PutMethodInput{
						RestAPIID:         api.ID,
						ResourceID:        rootID,
						HTTPMethod:        "GET",
						AuthorizationType: "NONE",
					},
				)

				return fmt.Sprintf("/restapis/%s/resources/%s/methods/GET", api.ID, rootID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := apigateway.NewInMemoryBackend()
			h := apigateway.NewHandler(backend)

			path := tt.path
			if tt.setup != nil {
				path = tt.setup(backend)
			}

			rec := restRequest(t, h, tt.method, path, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestExtractResource_AdditionalBranches covers the "name" key fallback and the
// non-string-value branch in ExtractResource.
func TestExtractResource_AdditionalBranches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantResource string
	}{
		{
			name:         "name_key_fallback",
			body:         `{"name":"my-api"}`,
			wantResource: "my-api",
		},
		{
			name:         "non_string_restApiId_falls_through_to_name",
			body:         `{"restApiId":42,"name":"fallback-api"}`,
			wantResource: "fallback-api",
		},
		{
			name:         "invalid_json_returns_empty",
			body:         `not-json`,
			wantResource: "",
		},
		{
			name:         "no_matching_keys",
			body:         `{"other":"value"}`,
			wantResource: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := apigateway.NewHandler(apigateway.NewInMemoryBackend())
			e := echo.New()

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			got := h.ExtractResource(e.NewContext(req, httptest.NewRecorder()))

			assert.Equal(t, tt.wantResource, got)
		})
	}
}

// TestHandler_GetSupportedOperations covers the `GET /` handler branch that returns
// the list of supported operations.
func TestHandler_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantCode int
	}{
		{
			name:     "GET_root_returns_operations",
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			h := apigateway.NewHandler(apigateway.NewInMemoryBackend())

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			err := h.Handler()(e.NewContext(req, rec))
			require.NoError(t, err)

			assert.Equal(t, tt.wantCode, rec.Code)

			var ops []string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ops))
			assert.Contains(t, ops, "CreateRestApi")
		})
	}
}

// TestHandler_InvalidTarget covers the branch that rejects an X-Amz-Target header
// that does not contain exactly one dot (e.g. "NoDotsHere").
func TestHandler_InvalidTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		target   string
		wantCode int
	}{
		{
			name:     "target_without_dot_returns_400",
			target:   "NoDotInTarget",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			h := apigateway.NewHandler(apigateway.NewInMemoryBackend())

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
			req.Header.Set("X-Amz-Target", tt.target)
			rec := httptest.NewRecorder()
			err := h.Handler()(e.NewContext(req, rec))
			require.NoError(t, err)

			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}
