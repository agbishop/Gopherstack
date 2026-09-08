package resourcegroups_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/services/resourcegroups"
)

// doResourceTagsRequestRaw sends a raw (possibly invalid/oversized) body to
// the /resources/{arn}/tags endpoint, unlike doResourceTagsRequest which
// always JSON-marshals a well-formed body.
func doResourceTagsRequestRaw(
	t *testing.T,
	h *resourcegroups.Handler,
	method, resourceARN string,
	body []byte,
) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	path := "/resources/" + resourceARN + "/tags"
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Request().RequestURI = path

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

// TestResourceGroupsHandler_Untag_RejectionNotBypassed guards
// extractUntagKeys' two error branches (ReadBody failure, JSON-unmarshal
// failure). Both used to write their own error body via h.handleError and
// return that call's (always-nil, on a successful write) result directly;
// handleUntagRequest tested that nil and fell through to call
// Backend.RemoveTagsByARN with keys == nil, then wrote a second 200 body on
// top of the already-committed error body (gopherstack-8haq shape). A nil
// keys slice is a genuine no-op (Tags.DeleteKeys ranges over it and deletes
// nothing), so no tag was ever actually removed -- but the response was
// still corrupted. echo guards the second WriteHeader, so the status code
// alone does not distinguish fixed from broken; the response body does.
func TestResourceGroupsHandler_Untag_RejectionNotBypassed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       []byte
		wantStatus int
	}{
		{
			name:       "malformed_json_body",
			body:       []byte("{not valid json"),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "oversized_body",
			body:       bytes.Repeat([]byte("x"), int(httputils.MaxRequestBodyBytes+1)),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestResourceGroupsHandler(t)

			rec := doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{
				"Name": "untag-rejection-group-" + tt.name,
				"Tags": map[string]string{"env": "prod"},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var created struct {
				Group struct {
					GroupArn string `json:"GroupArn"`
				} `json:"Group"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
			groupARN := created.Group.GroupArn

			untagRec := doResourceTagsRequestRaw(t, h, http.MethodDelete, groupARN, tt.body)

			assert.Equal(t, tt.wantStatus, untagRec.Code)

			var resp map[string]any
			require.NoError(
				t, json.Unmarshal(untagRec.Body.Bytes(), &resp),
				"response body must be a single well-formed JSON document, not two concatenated: %s",
				untagRec.Body.String(),
			)

			tagsRec := doResourceTagsRequest(t, h, http.MethodGet, groupARN, nil)
			require.Equal(t, http.StatusOK, tagsRec.Code)

			var tagsResp struct {
				Tags map[string]string `json:"Tags"`
			}
			require.NoError(t, json.Unmarshal(tagsRec.Body.Bytes(), &tagsResp))
			assert.Equal(t, map[string]string{"env": "prod"}, tagsResp.Tags, "rejected Untag must not remove any tags")
		})
	}
}
