package apigatewayv2_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/apigatewayv2"
)

func TestHandler_CreateIntegrationResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body          any
		name          string
		apiID         string
		integrationID string
		wantRespKey   string
		wantStatus    int
	}{
		{
			name:        "success",
			body:        map[string]any{"integrationResponseKey": "$default"},
			wantRespKey: "$default",
			wantStatus:  http.StatusCreated,
		},
		{
			name:       "api_not_found",
			apiID:      "nonexistent",
			body:       map[string]any{"integrationResponseKey": "$default"},
			wantStatus: http.StatusNotFound,
		},
		{
			name:          "integration_not_found",
			integrationID: "nonexistent",
			body:          map[string]any{"integrationResponseKey": "$default"},
			wantStatus:    http.StatusNotFound,
		},
		{
			name:       "invalid_body",
			body:       "not-json",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			apiID := tt.apiID
			integrationID := tt.integrationID

			if apiID == "" {
				apiID = createAPI(t, h, "test-api")
			}

			if integrationID == "" && tt.wantStatus != http.StatusBadRequest && tt.apiID == "" {
				integrationRR := doRequest(t, h, http.MethodPost,
					fmt.Sprintf("/v2/apis/%s/integrations", apiID),
					map[string]any{"integrationType": "HTTP_PROXY"})
				require.Equal(t, http.StatusCreated, integrationRR.Code)

				var integ apigatewayv2.Integration
				require.NoError(t, json.Unmarshal(integrationRR.Body.Bytes(), &integ))
				integrationID = integ.IntegrationID
			}

			if integrationID == "" {
				integrationID = "placeholder"
			}

			path := fmt.Sprintf("/v2/apis/%s/integrations/%s/integrationresponses", apiID, integrationID)

			var rr *httptest.ResponseRecorder

			if s, ok := tt.body.(string); ok {
				rr = doRequestRaw(t, h, path, s)
			} else {
				rr = doRequest(t, h, http.MethodPost, path, tt.body)
			}

			assert.Equal(t, tt.wantStatus, rr.Code)

			if tt.wantRespKey != "" {
				var ir apigatewayv2.IntegrationResponse
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &ir))
				assert.Equal(t, tt.wantRespKey, ir.IntegrationResponseKey)
				assert.NotEmpty(t, ir.IntegrationResponseID)
			}
		})
	}
}

func TestHandler_DuplicateIntegrationResponseKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{
			name:       "duplicate_key_returns_409",
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			apiID := createAPI(t, h, "test-api")

			rr := doRequest(t, h, http.MethodPost, fmt.Sprintf("/v2/apis/%s/integrations", apiID), map[string]any{
				"integrationType": "HTTP_PROXY",
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			var integration apigatewayv2.Integration
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integration))

			path := fmt.Sprintf("/v2/apis/%s/integrations/%s/integrationresponses", apiID, integration.IntegrationID)

			rr = doRequest(t, h, http.MethodPost, path, map[string]any{
				"integrationResponseKey": "$default",
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			rr = doRequest(t, h, http.MethodPost, path, map[string]any{
				"integrationResponseKey": "$default",
			})
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestHandler_GetIntegrationResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		apiExists         bool
		integrationExists bool
		responseCnt       int
		wantStatus        int
	}{
		{
			name:              "empty",
			apiExists:         true,
			integrationExists: true,
			responseCnt:       0,
			wantStatus:        http.StatusOK,
		},
		{
			name:              "one_response",
			apiExists:         true,
			integrationExists: true,
			responseCnt:       1,
			wantStatus:        http.StatusOK,
		},
		{
			name:              "api_not_found",
			apiExists:         false,
			integrationExists: false,
			wantStatus:        http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			apiID := "nonexistent"
			integrationID := "nonexistent"

			if tt.apiExists {
				apiID = createAPI(t, h, "test-api")
			}

			if tt.integrationExists {
				rr := doRequest(t, h, http.MethodPost, fmt.Sprintf("/v2/apis/%s/integrations", apiID), map[string]any{
					"integrationType": "HTTP_PROXY",
					"integrationUri":  "https://example.com",
				})
				require.Equal(t, http.StatusCreated, rr.Code)

				var integration apigatewayv2.Integration
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integration))
				integrationID = integration.IntegrationID
			}

			for i := range tt.responseCnt {
				path := fmt.Sprintf("/v2/apis/%s/integrations/%s/integrationresponses", apiID, integrationID)
				rr := doRequest(t, h, http.MethodPost, path, map[string]any{
					"integrationResponseKey": fmt.Sprintf("/2%d0/", i),
				})
				require.Equal(t, http.StatusCreated, rr.Code)
			}

			rr := doRequest(t, h, http.MethodGet,
				fmt.Sprintf("/v2/apis/%s/integrations/%s/integrationresponses", apiID, integrationID), nil)
			assert.Equal(t, tt.wantStatus, rr.Code)

			if tt.wantStatus == http.StatusOK {
				var out struct {
					Items []apigatewayv2.IntegrationResponse `json:"items"`
				}
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
				assert.Len(t, out.Items, tt.responseCnt)
			}
		})
	}
}

func TestHandler_GetIntegrationResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		useWrongID bool
		wantStatus int
	}{
		{
			name:       "found",
			wantStatus: http.StatusOK,
		},
		{
			name:       "not_found",
			useWrongID: true,
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			apiID := createAPI(t, h, "test-api")

			rr := doRequest(t, h, http.MethodPost, fmt.Sprintf("/v2/apis/%s/integrations", apiID), map[string]any{
				"integrationType": "HTTP_PROXY",
				"integrationUri":  "https://example.com",
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			var integration apigatewayv2.Integration
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integration))

			path := fmt.Sprintf("/v2/apis/%s/integrations/%s/integrationresponses", apiID, integration.IntegrationID)
			rr = doRequest(t, h, http.MethodPost, path, map[string]any{
				"integrationResponseKey": "$default",
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			var ir apigatewayv2.IntegrationResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &ir))

			responseID := ir.IntegrationResponseID
			if tt.useWrongID {
				responseID = "nonexistent"
			}

			rr = doRequest(t, h, http.MethodGet,
				fmt.Sprintf("/v2/apis/%s/integrations/%s/integrationresponses/%s",
					apiID, integration.IntegrationID, responseID), nil)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestHandler_DeleteIntegrationResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		useWrongID bool
		wantStatus int
	}{
		{
			name:       "success",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "not_found",
			useWrongID: true,
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			apiID := createAPI(t, h, "test-api")

			rr := doRequest(t, h, http.MethodPost, fmt.Sprintf("/v2/apis/%s/integrations", apiID), map[string]any{
				"integrationType": "HTTP_PROXY",
				"integrationUri":  "https://example.com",
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			var integration apigatewayv2.Integration
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integration))

			path := fmt.Sprintf("/v2/apis/%s/integrations/%s/integrationresponses", apiID, integration.IntegrationID)
			rr = doRequest(t, h, http.MethodPost, path, map[string]any{
				"integrationResponseKey": "$default",
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			var ir apigatewayv2.IntegrationResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &ir))

			responseID := ir.IntegrationResponseID
			if tt.useWrongID {
				responseID = "nonexistent"
			}

			rr = doRequest(t, h, http.MethodDelete,
				fmt.Sprintf("/v2/apis/%s/integrations/%s/integrationresponses/%s",
					apiID, integration.IntegrationID, responseID), nil)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestHandler_UpdateIntegrationResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *apigatewayv2.Handler) (apiID, integrationID, responseID string)
		body       any
		name       string
		wantStatus int
	}{
		{
			name: "success",
			setup: func(h *apigatewayv2.Handler) (string, string, string) {
				apiID := createAPI(t, h, "test-api")
				rr := doRequest(t, h, http.MethodPost, fmt.Sprintf("/v2/apis/%s/integrations", apiID), map[string]any{
					"integrationType": "HTTP_PROXY",
				})
				require.Equal(t, http.StatusCreated, rr.Code)
				var i apigatewayv2.Integration
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &i))

				rr = doRequest(t, h, http.MethodPost,
					fmt.Sprintf("/v2/apis/%s/integrations/%s/integrationresponses", apiID, i.IntegrationID),
					map[string]any{"integrationResponseKey": "200"})
				require.Equal(t, http.StatusCreated, rr.Code)
				var ir apigatewayv2.IntegrationResponse
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &ir))

				return apiID, i.IntegrationID, ir.IntegrationResponseID
			},
			body:       map[string]any{"contentHandlingStrategy": "CONVERT_TO_TEXT"},
			wantStatus: http.StatusOK,
		},
		{
			name: "not_found",
			setup: func(_ *apigatewayv2.Handler) (string, string, string) {
				return "nonexistent", "int123", "resp123"
			},
			body:       map[string]any{},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			apiID, integrationID, responseID := tt.setup(h)

			rr := doRequest(t, h, http.MethodPatch,
				fmt.Sprintf("/v2/apis/%s/integrations/%s/integrationresponses/%s", apiID, integrationID, responseID),
				tt.body)

			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}
