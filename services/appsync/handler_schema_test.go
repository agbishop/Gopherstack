package appsync_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appsync"
)

func TestHandler_StartSchemaCreation_Base64Encoded(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)

	sdl := `type Query { hello: String }`
	encoded := base64.StdEncoding.EncodeToString([]byte(sdl))

	body := map[string]any{"definition": encoded}
	rec := doRequest(t, h, http.MethodPost, "/v1/apis/"+api.APIID+"/schemacreation", body)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_StartSchemaCreation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sdl         string
		wantStatus2 string
		wantStatus  int
	}{
		{
			name:        "valid_schema_returns_active",
			sdl:         `type Query { hello: String }`,
			wantStatus:  http.StatusOK,
			wantStatus2: string(appsync.SchemaStatusActive),
		},
		{
			name:       "invalid_schema_returns_400",
			sdl:        `type { broken`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandler()
			api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)

			body := map[string]any{"definition": tt.sdl}
			rec := doRequest(t, h, http.MethodPost, "/v1/apis/"+api.APIID+"/schemacreation", body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus2 != "" {
				var resp map[string]any
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				assert.Equal(t, tt.wantStatus2, resp["status"])
			}
		})
	}
}

func TestHandler_GetSchemaCreationStatus(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { hello: String }`)

	rec := doRequest(t, h, http.MethodGet, "/v1/apis/"+api.APIID+"/schemacreation", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, string(appsync.SchemaStatusActive), resp["status"])
}

func TestHandler_GetIntrospectionSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		hasSchema  bool
		wantStatus int
	}{
		{
			name:       "returns_schema_sdl",
			hasSchema:  true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "returns_404_when_no_schema",
			hasSchema:  false,
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandler()
			api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)

			if tt.hasSchema {
				_, _ = b.StartSchemaCreation(api.APIID, `type Query { hello: String }`)
			}

			rec := doRequest(t, h, http.MethodGet, "/v1/apis/"+api.APIID+"/schema", nil)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.hasSchema {
				assert.Equal(t, `type Query { hello: String }`, rec.Body.String())
			}
		})
	}
}

func TestHandler_GetIntrospectionSchema_Format(t *testing.T) {
	t.Parallel()

	tests := []struct {
		checkBody  func(t *testing.T, body string)
		name       string
		query      string
		wantStatus int
	}{
		{
			name:       "explicit_sdl_returns_raw_sdl",
			query:      "?format=SDL",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body string) {
				t.Helper()
				assert.Equal(t, `type Query { hello: String }`, body)
			},
		},
		{
			name:       "json_returns_introspection_document",
			query:      "?format=JSON",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, body string) {
				t.Helper()

				require.True(t, json.Valid([]byte(body)))

				var doc map[string]any
				require.NoError(t, json.Unmarshal([]byte(body), &doc))

				data, ok := doc["data"].(map[string]any)
				require.True(t, ok)
				schema, ok := data["__schema"].(map[string]any)
				require.True(t, ok)
				queryType, ok := schema["queryType"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "Query", queryType["name"])
			},
		},
		{
			name:       "unrecognized_format_is_bad_request",
			query:      "?format=XML",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandler()
			api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
			_, err := b.StartSchemaCreation(api.APIID, `type Query { hello: String }`)
			require.NoError(t, err)

			rec := doRequest(t, h, http.MethodGet, "/v1/apis/"+api.APIID+"/schema"+tt.query, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.checkBody != nil {
				tt.checkBody(t, rec.Body.String())
			}
		})
	}
}

// TestHandler_GetIntrospectionSchema_InvalidSchema_JSON proves that when the
// stored schema failed to parse and format=JSON is requested, the handler
// emits GraphQLSchemaException on the wire -- GetIntrospectionSchema's
// declared error set (appsync@v1.56.4 deserializers.go) has no
// BadRequestException member, unlike StartSchemaCreation's (gopherstack-w4kf).
func TestHandler_GetIntrospectionSchema_InvalidSchema_JSON(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)

	_, err := b.StartSchemaCreation(api.APIID, `type { broken schema`)
	require.Error(t, err)

	rec := doRequest(t, h, http.MethodGet, "/v1/apis/"+api.APIID+"/schema?format=JSON", nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "GraphQLSchemaException", resp["code"])
}

func TestHandler_SchemaCreations_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)

	rec := doRequest(t, h, http.MethodPut, "/v1/apis/"+api.APIID+"/schemacreation", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
