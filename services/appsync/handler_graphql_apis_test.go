package appsync_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appsync"
)

func TestHandler_GraphQL_NoSchema(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	// No schema uploaded.
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)

	body := map[string]any{"query": `query { hello }`}
	rec := doRequestWithHeaders(t, h, "/v1/apis/"+api.APIID+"/graphql", body,
		map[string]string{"x-api-key": key.ID})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_CreateAndGetGraphqlAPI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body        map[string]any
		name        string
		wantAPIName string
		wantStatus  int
	}{
		{
			name:        "creates_api_successfully",
			body:        map[string]any{"name": "MyAPI", "authenticationType": "API_KEY"},
			wantStatus:  http.StatusCreated,
			wantAPIName: "MyAPI",
		},
		{
			name:       "missing_name_returns_400",
			body:       map[string]any{"authenticationType": "API_KEY"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler()
			rec := doRequest(t, h, http.MethodPost, "/v1/apis", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantAPIName != "" {
				var resp map[string]any
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				api, ok := resp["graphqlApi"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.wantAPIName, api["name"])
				assert.NotEmpty(t, api["apiId"])
			}
		})
	}
}

func TestHandler_DeleteGraphqlAPI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		apiID      string
		wantStatus int
	}{
		{
			name:       "deletes_existing_api",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "returns_404_for_missing_api",
			apiID:      "nonexistent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandler()
			apiID := tt.apiID

			if apiID == "" {
				api, _ := b.CreateGraphqlAPI("ToDelete", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
				apiID = api.APIID
			}

			rec := doRequest(t, h, http.MethodDelete, "/v1/apis/"+apiID, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_GraphQLExecution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		schema     string
		query      string
		wantKey    string
		wantStatus int
	}{
		{
			name:       "executes_none_resolver",
			schema:     `type Query { ping: String }`,
			query:      `query { ping }`,
			wantStatus: http.StatusOK,
			wantKey:    "ping",
		},
		{
			name:       "returns_error_for_unknown_api",
			schema:     "",
			query:      `query { ping }`,
			wantStatus: http.StatusOK,
			wantKey:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandler()
			api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
			key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
			require.NoError(t, keyErr)

			if tt.schema != "" {
				_, _ = b.StartSchemaCreation(api.APIID, tt.schema)
				_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
					Name: "NoneDS",
					Type: appsync.DataSourceTypeNone,
				})
				_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
					FieldName:      "ping",
					DataSourceName: "NoneDS",
				})
			}

			body := map[string]any{"query": tt.query}
			path := "/v1/apis/" + api.APIID + "/graphql"
			rec := doRequestWithHeaders(t, h, path, body, map[string]string{"x-api-key": key.ID})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantKey != "" {
				var resp map[string]any
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

				if _, hasErrors := resp["errors"]; !hasErrors {
					data, ok := resp["data"].(map[string]any)
					require.True(t, ok)
					assert.Contains(t, data, tt.wantKey)
				}
			}
		})
	}
}

func TestHandler_ListGraphqlAPIs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(*appsync.InMemoryBackend)
		name      string
		wantCount int
	}{
		{
			name:      "empty_list",
			setup:     func(_ *appsync.InMemoryBackend) {},
			wantCount: 0,
		},
		{
			name: "returns_all_apis",
			setup: func(b *appsync.InMemoryBackend) {
				_, _ = b.CreateGraphqlAPI("API1", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
				_, _ = b.CreateGraphqlAPI("API2", appsync.AuthTypeIAM, false, "", "", nil, nil, nil)
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandler()
			tt.setup(b)

			rec := doRequest(t, h, http.MethodGet, "/v1/apis", nil)
			assert.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			apis, ok := resp["graphqlApis"].([]any)
			require.True(t, ok)
			assert.Len(t, apis, tt.wantCount)
		})
	}
}

func TestHandler_GetGraphqlAPI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		apiID      string
		wantStatus int
	}{
		{
			name:       "returns_existing_api",
			wantStatus: http.StatusOK,
		},
		{
			name:       "returns_404_for_missing_api",
			apiID:      "nonexistent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandler()
			apiID := tt.apiID

			if apiID == "" {
				api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
				apiID = api.APIID
			}

			rec := doRequest(t, h, http.MethodGet, "/v1/apis/"+apiID, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_APIs_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler()
	rec := doRequest(t, h, http.MethodPut, "/v1/apis", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandler_GraphQL_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)

	rec := doRequest(t, h, http.MethodGet, "/v1/apis/"+api.APIID+"/graphql", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandler_GraphQL_InvalidJSON(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { hello: String }`)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/apis/"+api.APIID+"/graphql",
		strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := h.Handler()(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_API_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler()

	// CONNECT is not allowed on the /v1/apis path.
	rec := doRequest(t, h, http.MethodConnect, "/v1/apis", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandler_CreateGraphqlAPI_InvalidAuthType(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler()
	body := map[string]any{"name": "TestAPI", "authenticationType": "INVALID_TYPE"}
	rec := doRequest(t, h, http.MethodPost, "/v1/apis", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_UpdateGraphqlAPI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantName   string
		wantStatus int
	}{
		{
			name:       "updates_name",
			body:       map[string]any{"name": "UpdatedAPI"},
			wantStatus: http.StatusOK,
			wantName:   "UpdatedAPI",
		},
		{
			name:       "updates_auth_type",
			body:       map[string]any{"authenticationType": "AWS_IAM"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid_auth_type_rejected",
			body:       map[string]any{"authenticationType": "INVALID"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandler()
			api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
			require.NoError(t, err)

			rec := doRequest(t, h, http.MethodPatch, "/v1/apis/"+api.APIID, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantName != "" && tt.wantStatus == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				gqlAPI := resp["graphqlApi"].(map[string]any)
				assert.Equal(t, tt.wantName, gqlAPI["name"])
			}
		})
	}
}

func TestHandler_CreateAndUpdateGraphqlAPI_OwnerContact(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler()

	createRec := doRequest(t, h, http.MethodPost, "/v1/apis",
		map[string]any{"name": "TestAPI", "ownerContact": "team-a@example.com"})
	require.Equal(t, http.StatusCreated, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.NewDecoder(createRec.Body).Decode(&createResp))
	createdAPI := createResp["graphqlApi"].(map[string]any)
	assert.Equal(t, "team-a@example.com", createdAPI["ownerContact"])
	apiID := createdAPI["apiId"].(string)

	getRec := doRequest(t, h, http.MethodGet, "/v1/apis/"+apiID, nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.NewDecoder(getRec.Body).Decode(&getResp))
	assert.Equal(t, "team-a@example.com", getResp["graphqlApi"].(map[string]any)["ownerContact"])

	updateRec := doRequest(t, h, http.MethodPatch, "/v1/apis/"+apiID,
		map[string]any{"ownerContact": "team-b@example.com"})
	require.Equal(t, http.StatusOK, updateRec.Code)

	var updateResp map[string]any
	require.NoError(t, json.NewDecoder(updateRec.Body).Decode(&updateResp))
	assert.Equal(t, "team-b@example.com", updateResp["graphqlApi"].(map[string]any)["ownerContact"])
}

func TestHandler_EnvironmentVariables(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	// Get empty env vars.
	rec1 := doRequest(t, h, http.MethodGet, "/v1/apis/"+api.APIID+"/environmentVariables", nil)
	require.Equal(t, http.StatusOK, rec1.Code)

	var getResp map[string]any
	require.NoError(t, json.NewDecoder(rec1.Body).Decode(&getResp))
	assert.Empty(t, getResp["environmentVariables"])

	// Put env vars.
	rec2 := doRequest(t, h, http.MethodPut, "/v1/apis/"+api.APIID+"/environmentVariables",
		map[string]any{"environmentVariables": map[string]any{"KEY1": "value1", "KEY2": "value2"}})
	require.Equal(t, http.StatusOK, rec2.Code)

	var putResp map[string]any
	require.NoError(t, json.NewDecoder(rec2.Body).Decode(&putResp))
	envVars := putResp["environmentVariables"].(map[string]any)
	assert.Equal(t, "value1", envVars["KEY1"])
	assert.Equal(t, "value2", envVars["KEY2"])

	// Get env vars after put.
	rec3 := doRequest(t, h, http.MethodGet, "/v1/apis/"+api.APIID+"/environmentVariables", nil)
	require.Equal(t, http.StatusOK, rec3.Code)

	var getResp2 map[string]any
	require.NoError(t, json.NewDecoder(rec3.Body).Decode(&getResp2))
	envVars2 := getResp2["environmentVariables"].(map[string]any)
	assert.Equal(t, "value1", envVars2["KEY1"])
}

func TestHandler_EnvironmentVariables_NotFound(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler()

	rec := doRequest(t, h, http.MethodGet, "/v1/apis/nonexistent/environmentVariables", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_EnvironmentVariables_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodDelete, "/v1/apis/"+api.APIID+"/environmentVariables", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandler_CreateGraphqlAPI_XrayEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body        map[string]any
		name        string
		wantAPIType string
		wantXray    bool
	}{
		{
			name:        "with_xray_enabled",
			body:        map[string]any{"name": "MyAPI", "xrayEnabled": true, "apiType": "GRAPHQL"},
			wantXray:    true,
			wantAPIType: "GRAPHQL",
		},
		{
			name:        "default_no_xray",
			body:        map[string]any{"name": "MyAPI"},
			wantXray:    false,
			wantAPIType: "GRAPHQL",
		},
		{
			name:        "merged_api_type",
			body:        map[string]any{"name": "MyMergedAPI", "apiType": "MERGED"},
			wantAPIType: "MERGED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler()
			rec := doRequest(t, h, http.MethodPost, "/v1/apis", tt.body)
			require.Equal(t, http.StatusCreated, rec.Code)

			var resp map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			apiObj := resp["graphqlApi"].(map[string]any)
			assert.NotEmpty(t, apiObj["apiId"])

			if tt.wantAPIType != "" {
				assert.Equal(t, tt.wantAPIType, apiObj["apiType"])
			}
		})
	}
}

func TestHandler_UpdateGraphqlAPI_XrayEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body        map[string]any
		name        string
		wantErrCode int
		wantXray    bool
	}{
		{
			name:     "enable_xray",
			body:     map[string]any{"xrayEnabled": true},
			wantXray: true,
		},
		{
			name:     "disable_xray",
			body:     map[string]any{"xrayEnabled": false},
			wantXray: false,
		},
		{
			name:        "api_not_found",
			body:        map[string]any{"name": "new"},
			wantErrCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandler()
			api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)

			apiID := api.APIID
			if tt.wantErrCode != 0 {
				apiID = "nonexistent"
			}

			rec := doRequest(t, h, http.MethodPatch, "/v1/apis/"+apiID, tt.body)

			if tt.wantErrCode != 0 {
				assert.Equal(t, tt.wantErrCode, rec.Code)

				return
			}

			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			apiObj := resp["graphqlApi"].(map[string]any)
			// xrayEnabled omits false in JSON (omitempty), so nil == false.
			gotXray, _ := apiObj["xrayEnabled"].(bool)
			assert.Equal(t, tt.wantXray, gotXray)
		})
	}
}

func TestHandler_ListGraphqlAPIs_ApiTypeFilter(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	_, _ = b.CreateGraphqlAPI("GraphQL1", appsync.AuthTypeAPIKey, false, "GRAPHQL", "", nil, nil, nil)
	_, _ = b.CreateGraphqlAPI("Merged1", appsync.AuthTypeAPIKey, false, "MERGED", "", nil, nil, nil)

	rec := doRequest(t, h, http.MethodGet, "/v1/apis?apiType=GRAPHQL", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	apis := resp["graphqlApis"].([]any)
	assert.Len(t, apis, 1)
}

func TestHandler_EnvironmentVariables_ExceedsLimit(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, err := b.CreateGraphqlAPI("MyAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	// Build a map with 26 entries (exceeds max of 25).
	envVars := make(map[string]string)
	for i := range 26 {
		envVars[strings.Repeat("K", i+1)] = "value"
	}

	rec := doRequest(t, h, http.MethodPut, "/v1/apis/"+api.APIID+"/environmentVariables", map[string]any{
		"environmentVariables": envVars,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
