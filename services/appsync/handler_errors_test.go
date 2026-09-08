package appsync_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appsync"
)

func TestHandler_HandleError_InternalError(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	key, keyErr := b.CreateAPIKey(api.APIID, "", 0)
	require.NoError(t, keyErr)

	// Schema with unsupported data source causes InternalFailure.
	_, _ = b.StartSchemaCreation(api.APIID, `type Query { hello: String }`)
	_, _ = b.CreateDataSource(api.APIID, &appsync.DataSource{
		Name: "HTTPDS",
		Type: appsync.DataSourceTypeHTTP,
	})
	_, _ = b.CreateResolver(api.APIID, "Query", &appsync.Resolver{
		FieldName:      "hello",
		DataSourceName: "HTTPDS",
	})

	body := map[string]any{"query": `query { hello }`}
	rec := doRequestWithHeaders(t, h, "/v1/apis/"+api.APIID+"/graphql", body,
		map[string]string{"x-api-key": key.ID})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_Unknown_Short_Segs(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/unknown", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := h.Handler()(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_UnknownSubpath(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler()
	api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)

	rec := doRequest(t, h, http.MethodGet, "/v1/apis/"+api.APIID+"/unknown", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
