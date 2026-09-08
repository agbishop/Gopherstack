package apigatewayv2_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/apigatewayv2"
)

func TestHandler_CreateIntegration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		integrationType string
		wantStatus      int
		apiExists       bool
	}{
		{
			name:            "success",
			integrationType: "AWS_PROXY",
			wantStatus:      http.StatusCreated,
			apiExists:       true,
		},
		{
			name:            "api_not_found",
			integrationType: "AWS_PROXY",
			wantStatus:      http.StatusNotFound,
			apiExists:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			apiID := "nonexistent"
			if tt.apiExists {
				apiID = createAPI(t, h, "test-api")
			}

			rr := doRequest(t, h, http.MethodPost, fmt.Sprintf("/v2/apis/%s/integrations", apiID), map[string]any{
				"integrationType": tt.integrationType,
			})

			assert.Equal(t, tt.wantStatus, rr.Code)

			if tt.wantStatus == http.StatusCreated {
				var integration apigatewayv2.Integration
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integration))
				assert.Equal(t, tt.integrationType, integration.IntegrationType)
				assert.NotEmpty(t, integration.IntegrationID)
			}
		})
	}
}

func TestHandler_GetIntegrations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		integrations []string
		wantStatus   int
		apiExists    bool
	}{
		{
			name:         "empty",
			integrations: nil,
			wantStatus:   http.StatusOK,
			apiExists:    true,
		},
		{
			name:         "multiple",
			integrations: []string{"AWS_PROXY", "HTTP_PROXY"},
			wantStatus:   http.StatusOK,
			apiExists:    true,
		},
		{
			name:       "api_not_found",
			wantStatus: http.StatusNotFound,
			apiExists:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			apiID := "nonexistent"
			if tt.apiExists {
				apiID = createAPI(t, h, "test-api")

				for _, it := range tt.integrations {
					rr := doRequest(
						t,
						h,
						http.MethodPost,
						fmt.Sprintf("/v2/apis/%s/integrations", apiID),
						map[string]any{
							"integrationType": it,
						},
					)
					require.Equal(t, http.StatusCreated, rr.Code)
				}
			}

			rr := doRequest(t, h, http.MethodGet, fmt.Sprintf("/v2/apis/%s/integrations", apiID), nil)
			assert.Equal(t, tt.wantStatus, rr.Code)

			if tt.wantStatus == http.StatusOK {
				type listResp struct {
					Items []apigatewayv2.Integration `json:"items"`
				}

				var resp listResp
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
				assert.Len(t, resp.Items, len(tt.integrations))
			}
		})
	}
}

func TestHandler_GetIntegration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		wantStatus       int
		setupIntegration bool
	}{
		{
			name:             "existing",
			wantStatus:       http.StatusOK,
			setupIntegration: true,
		},
		{
			name:             "not_found",
			wantStatus:       http.StatusNotFound,
			setupIntegration: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			apiID := createAPI(t, h, "test-api")

			integrationID := "nonexistent"
			if tt.setupIntegration {
				rr := doRequest(t, h, http.MethodPost, fmt.Sprintf("/v2/apis/%s/integrations", apiID), map[string]any{
					"integrationType": "AWS_PROXY",
				})
				require.Equal(t, http.StatusCreated, rr.Code)

				var integration apigatewayv2.Integration
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integration))
				integrationID = integration.IntegrationID
			}

			rr := doRequest(t, h, http.MethodGet, fmt.Sprintf("/v2/apis/%s/integrations/%s", apiID, integrationID), nil)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestHandler_DeleteIntegration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		wantStatus       int
		setupIntegration bool
	}{
		{
			name:             "success",
			wantStatus:       http.StatusNoContent,
			setupIntegration: true,
		},
		{
			name:             "not_found",
			wantStatus:       http.StatusNotFound,
			setupIntegration: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			apiID := createAPI(t, h, "test-api")

			integrationID := "nonexistent"
			if tt.setupIntegration {
				rr := doRequest(t, h, http.MethodPost, fmt.Sprintf("/v2/apis/%s/integrations", apiID), map[string]any{
					"integrationType": "AWS_PROXY",
				})
				require.Equal(t, http.StatusCreated, rr.Code)

				var integration apigatewayv2.Integration
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integration))
				integrationID = integration.IntegrationID
			}

			rr := doRequest(
				t,
				h,
				http.MethodDelete,
				fmt.Sprintf("/v2/apis/%s/integrations/%s", apiID, integrationID),
				nil,
			)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestHandler_UpdateIntegration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		wantStatus       int
		setupIntegration bool
	}{
		{
			name:             "success",
			wantStatus:       http.StatusOK,
			setupIntegration: true,
		},
		{
			name:             "not_found",
			wantStatus:       http.StatusNotFound,
			setupIntegration: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			apiID := createAPI(t, h, "test-api")

			integrationID := "nonexistent"
			if tt.setupIntegration {
				rr := doRequest(t, h, http.MethodPost, fmt.Sprintf("/v2/apis/%s/integrations", apiID), map[string]any{
					"integrationType": "AWS_PROXY",
				})
				require.Equal(t, http.StatusCreated, rr.Code)

				var integration apigatewayv2.Integration
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integration))
				integrationID = integration.IntegrationID
			}

			rr := doRequest(
				t,
				h,
				http.MethodPatch,
				fmt.Sprintf("/v2/apis/%s/integrations/%s", apiID, integrationID),
				map[string]any{
					"integrationType": "HTTP_PROXY",
				},
			)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

// TestHandler_CreateIntegration_InvalidType also proves the per-protocol
// integrationType restriction api_op_CreateIntegration.go documents: AWS,
// HTTP, and MOCK are each "Supported only for WebSocket APIs", so only
// AWS_PROXY/HTTP_PROXY are valid on an HTTP API. Before this fix, buildIntegration's
// validTypes check ignored the API's protocolType entirely, so an HTTP API
// accepted AWS/HTTP/MOCK and only failed opaquely (500) at invoke time.
func TestHandler_CreateIntegration_InvalidType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body         map[string]any
		name         string
		protocolType string
		wantStatus   int
	}{
		{
			name:         "http_api_rejects_http_type",
			protocolType: "HTTP",
			body:         map[string]any{"integrationType": "HTTP"},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "http_api_rejects_mock",
			protocolType: "HTTP",
			body:         map[string]any{"integrationType": "MOCK"},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "http_api_rejects_aws",
			protocolType: "HTTP",
			body:         map[string]any{"integrationType": "AWS"},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "http_api_accepts_aws_proxy",
			protocolType: "HTTP",
			body:         map[string]any{"integrationType": "AWS_PROXY"},
			wantStatus:   http.StatusCreated,
		},
		{
			name:         "websocket_api_accepts_http",
			protocolType: "WEBSOCKET",
			body:         map[string]any{"integrationType": "HTTP"},
			wantStatus:   http.StatusCreated,
		},
		{
			name:         "websocket_api_accepts_mock",
			protocolType: "WEBSOCKET",
			body:         map[string]any{"integrationType": "MOCK"},
			wantStatus:   http.StatusCreated,
		},
		{
			name:         "invalid_type",
			protocolType: "HTTP",
			body:         map[string]any{"integrationType": "INVALID"},
			wantStatus:   http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			rr := doRequest(t, h, http.MethodPost, "/v2/apis", map[string]any{
				"name": "test-api", "protocolType": tt.protocolType,
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			var api apigatewayv2.API
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &api))

			rr = doRequest(t, h, http.MethodPost, fmt.Sprintf("/v2/apis/%s/integrations", api.APIID), tt.body)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestGetIntegrations_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	apiID := createAPI(t, h, "intg-api")

	for i := range 4 {
		rr := doRequest(t, h, http.MethodPost,
			fmt.Sprintf("/v2/apis/%s/integrations", apiID),
			map[string]any{
				"integrationType": "HTTP_PROXY",
				"integrationUri":  fmt.Sprintf("https://example.com/%d", i),
			},
		)
		require.Equal(t, http.StatusCreated, rr.Code)
	}

	tests := []struct {
		name       string
		maxResults int
		wantPages  int
	}{
		{"two_per_page", 2, 2},
		{"all_at_once", 10, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			seen := map[string]int{}
			nextToken := ""
			pages := 0

			for {
				path := fmt.Sprintf("/v2/apis/%s/integrations?maxResults=%d", apiID, tc.maxResults)
				if nextToken != "" {
					path += "&nextToken=" + nextToken
				}

				rr := doRequest(t, h, http.MethodGet, path, nil)
				require.Equal(t, http.StatusOK, rr.Code)

				var resp struct {
					NextToken string           `json:"nextToken"`
					Items     []map[string]any `json:"items"`
				}
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
				require.LessOrEqual(t, len(resp.Items), tc.maxResults)

				for _, s := range resp.Items {
					id, _ := s["integrationId"].(string)
					seen[id]++
				}

				pages++
				nextToken = resp.NextToken

				if nextToken == "" {
					break
				}

				require.Less(t, pages, 20)
			}

			assert.Equal(t, tc.wantPages, pages)
			assert.Len(t, seen, 4)

			for id, count := range seen {
				assert.Equalf(t, 1, count, "integration %s duplicated", id)
			}
		})
	}
}
