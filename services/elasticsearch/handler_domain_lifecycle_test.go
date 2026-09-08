package elasticsearch_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/elasticsearch"
)

func TestElasticsearchHandler_CancelElasticsearchServiceSoftwareUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *elasticsearch.Handler)
		name         string
		domainName   string
		wantContains []string
		wantCode     int
	}{
		{
			name:       "success",
			domainName: "my-domain",
			setup: func(t *testing.T, h *elasticsearch.Handler) {
				t.Helper()
				r := doRequest(t, h, http.MethodPost, "/2015-01-01/es/domain", map[string]any{
					"DomainName": "my-domain",
				})
				r.Body.Close()
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"ServiceSoftwareOptions", "NOT_ELIGIBLE"},
		},
		{
			name:       "domain_not_found",
			domainName: "nonexistent",
			wantCode:   http.StatusNotFound,
		},
		{
			name:     "invalid_json",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			if tt.setup != nil {
				tt.setup(t, h)
			}

			if tt.name == "invalid_json" {
				req := httptest.NewRequest(http.MethodPost,
					"/2015-01-01/es/serviceSoftwareUpdate/cancel",
					strings.NewReader("not-json"))
				req.Header.Set("Content-Type", "application/json")
				rw := httptest.NewRecorder()
				h.ServeHTTP(rw, req)
				assert.Equal(t, tt.wantCode, rw.Code)

				return
			}

			resp := doRequest(t, h, http.MethodPost, "/2015-01-01/es/serviceSoftwareUpdate/cancel",
				map[string]any{"DomainName": tt.domainName})
			defer resp.Body.Close()

			assert.Equal(t, tt.wantCode, resp.StatusCode)

			if len(tt.wantContains) > 0 {
				bodyBytes, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				for _, s := range tt.wantContains {
					assert.Contains(t, string(bodyBytes), s)
				}
			}
		})
	}
}

func TestElasticsearchHandler_DeleteElasticsearchServiceRole(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	resp := doRequest(t, h, http.MethodDelete, "/2015-01-01/es/role", nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestElasticsearchHandler_DeleteElasticsearchServiceRole_RejectedWithVPCDomain
// locks real AWS's DeleteElasticsearchServiceRole doc comment: "Role
// deletion will fail if any existing VPC domains use the role. You must
// delete any such Elasticsearch domains before deleting the role".
func TestElasticsearchHandler_DeleteElasticsearchServiceRole_RejectedWithVPCDomain(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	createResp := doRequest(t, h, http.MethodPost, "/2015-01-01/es/domain", map[string]any{
		"DomainName": "vpc-role-domain",
		"VPCOptions": map[string]any{"SubnetIds": []string{"subnet-abc"}},
	})
	createResp.Body.Close()
	require.Equal(t, http.StatusOK, createResp.StatusCode)

	rejected := doRequest(t, h, http.MethodDelete, "/2015-01-01/es/role", nil)
	defer rejected.Body.Close()
	assert.NotEqual(t, http.StatusOK, rejected.StatusCode)

	delDomainResp := doRequest(t, h, http.MethodDelete, "/2015-01-01/es/domain/vpc-role-domain", nil)
	delDomainResp.Body.Close()
	require.Equal(t, http.StatusOK, delDomainResp.StatusCode)

	allowed := doRequest(t, h, http.MethodDelete, "/2015-01-01/es/role", nil)
	defer allowed.Body.Close()
	assert.Equal(t, http.StatusOK, allowed.StatusCode)
}

// TestElasticsearchHandler_UpgradeAndSoftwareUpdate_Lifecycle drives
// UpgradeElasticsearchDomain and StartElasticsearchServiceSoftwareUpdate
// through the HTTP handler.
func TestElasticsearchHandler_UpgradeAndSoftwareUpdate_Lifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	createTestDomainName(t, h, "upgrade-state-domain")

	resp := doRequest(t, h, http.MethodPost, "/2015-01-01/es/upgradeDomain", map[string]any{
		"DomainName": "upgrade-state-domain", "TargetVersion": "7.11",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = doRequest(t, h, http.MethodGet, "/2015-01-01/es/domain/upgrade-state-domain", nil)
	assert.Equal(t, "7.11", readJSONBody(t, resp)["DomainStatus"].(map[string]any)["ElasticsearchVersion"])

	resp = doRequest(t, h, http.MethodPost, "/2015-01-01/es/serviceSoftwareUpdate/start", map[string]any{
		"DomainName": "missing-domain",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}
