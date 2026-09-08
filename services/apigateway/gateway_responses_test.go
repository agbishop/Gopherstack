package apigateway_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/apigateway"
)

func TestAPIGateway_UpdateGatewayResponse_RESTRoute(t *testing.T) {
	t.Parallel()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)

	api, err := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "gwr-api"})
	require.NoError(t, err)

	_, err = backend.PutGatewayResponse(apigateway.PutGatewayResponseInput{
		RestAPIID: api.ID, ResponseType: "DEFAULT_4XX", StatusCode: "400",
	})
	require.NoError(t, err)

	rec := restCall(
		t, h, http.MethodPatch, "/restapis/"+api.ID+"/gatewayresponses/DEFAULT_4XX", "application/json",
		`[{"op":"replace","path":"/statusCode","value":"404"}]`,
	)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "404", resp["statusCode"])
}

// TestGatewayResponses tests GetGatewayResponse, GetGatewayResponses, PutGatewayResponse, DeleteGatewayResponse.
func TestGatewayResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		responseType    string
		putStatusCode   string
		wantPutCode     int
		wantGetCode     int
		wantGetListCode int
		wantDeleteCode  int
		wantDefaultResp bool
		doDelete        bool
		wantAfterDelete bool
		apiNotFound     bool
	}{
		{
			name:            "default_response_returned_when_not_set",
			responseType:    "UNAUTHORIZED",
			wantGetCode:     http.StatusOK,
			wantDefaultResp: true,
		},
		{
			name:            "put_and_get_custom",
			responseType:    "RESOURCE_NOT_FOUND",
			putStatusCode:   "404",
			wantPutCode:     http.StatusCreated,
			wantGetCode:     http.StatusOK,
			wantGetListCode: http.StatusOK,
		},
		{
			name:            "put_and_delete",
			responseType:    "THROTTLED",
			putStatusCode:   "429",
			wantPutCode:     http.StatusCreated,
			doDelete:        true,
			wantDeleteCode:  http.StatusNoContent,
			wantAfterDelete: true,
		},
		{
			name:            "api_not_found_get_list",
			responseType:    "UNAUTHORIZED",
			wantGetListCode: http.StatusNotFound,
			apiNotFound:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, e := boostSetup()
			apiID := boostAPI(t, handler, e)

			lookupAPIID := apiID
			if tt.apiNotFound {
				lookupAPIID = "notexist"
			}

			if tt.wantPutCode != 0 {
				putRec := postWithHandler(t, handler, e, "PutGatewayResponse",
					fmt.Sprintf(`{"restApiId":%q,"responseType":%q,"statusCode":%q}`,
						apiID, tt.responseType, tt.putStatusCode))
				assert.Equal(t, tt.wantPutCode, putRec.Code)
			}

			if tt.wantGetCode != 0 {
				getRec := postWithHandler(t, handler, e, "GetGatewayResponse",
					fmt.Sprintf(`{"restApiId":%q,"responseType":%q}`, apiID, tt.responseType))
				assert.Equal(t, tt.wantGetCode, getRec.Code)

				if tt.wantDefaultResp {
					var resp map[string]any
					require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &resp))
					assert.Equal(t, true, resp["defaultResponse"])
				}
			}

			if tt.wantGetListCode != 0 {
				listRec := postWithHandler(t, handler, e, "GetGatewayResponses",
					fmt.Sprintf(`{"restApiId":%q}`, lookupAPIID))
				assert.Equal(t, tt.wantGetListCode, listRec.Code)
			}

			if tt.doDelete {
				delRec := postWithHandler(t, handler, e, "DeleteGatewayResponse",
					fmt.Sprintf(`{"restApiId":%q,"responseType":%q}`, apiID, tt.responseType))
				assert.Equal(t, tt.wantDeleteCode, delRec.Code)

				if tt.wantAfterDelete {
					getRec := postWithHandler(t, handler, e, "GetGatewayResponse",
						fmt.Sprintf(`{"restApiId":%q,"responseType":%q}`, apiID, tt.responseType))
					assert.Equal(t, http.StatusOK, getRec.Code)
					var resp map[string]any
					require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &resp))
					// After delete, should revert to default
					assert.Equal(t, true, resp["defaultResponse"])
				}
			}
		})
	}
}

// TestDeleteRestAPI_ClearsGatewayResponses verifies that deleting a REST API
// removes its custom gateway responses rather than leaking them in the
// persisted snapshot under a restApiId no client can ever address again.
func TestDeleteRestAPI_ClearsGatewayResponses(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()

	api, err := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "gwr-cascade-api"})
	require.NoError(t, err)

	_, err = b.PutGatewayResponse(apigateway.PutGatewayResponseInput{
		RestAPIID: api.ID, ResponseType: "DEFAULT_4XX", StatusCode: "400",
	})
	require.NoError(t, err)

	require.NoError(t, b.DeleteRestAPI(api.ID))

	var decoded struct {
		Tables struct {
			GatewayResponses []struct {
				RestAPIID string `json:"restApiId"`
			} `json:"gatewayResponses"`
		} `json:"tables"`
	}
	require.NoError(t, json.Unmarshal(b.Snapshot(t.Context()), &decoded))
	for _, gr := range decoded.Tables.GatewayResponses {
		assert.NotEqual(t, api.ID, gr.RestAPIID,
			"a deleted REST API's gateway responses must not survive in the persisted snapshot")
	}
}
