package codeartifact_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandler_PackageVersionOps_RejectionNotBypassed guards
// validatePackageVersionParams' rejection path for GetPackageVersionReadme,
// ListPackageVersionAssets, and ListPackageVersionDependencies. All three call
// validatePackageVersionParams and check "if err != nil { return err }", but
// validatePackageVersionParams wrote its own 400 body via c.JSON and returned
// that call's (always-nil, on a successful write) result -- so the check never
// fired, and the handler fell through to call the real read-only Backend
// method with an empty parameter, which then wrote a second body (404, package
// version not found) on top of the already-committed 400. echo guards the
// second WriteHeader, so the status code alone does not distinguish fixed
// from broken -- the response body does: concatenated invalid JSON pre-fix,
// a single well-formed ValidationException body post-fix.
func TestHandler_PackageVersionOps_RejectionNotBypassed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{
			name: "get_readme_missing_version",
			path: "/v1/package/version/readme?domain=d&repository=r&format=npm&package=p",
		},
		{
			name: "list_assets_missing_version",
			path: "/v1/package/version/assets?domain=d&repository=r&format=npm&package=p",
		},
		{
			name: "list_dependencies_missing_version",
			path: "/v1/package/version/dependencies?domain=d&repository=r&format=npm&package=p",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(t, h, http.MethodGet, tt.path, nil)

			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var resp map[string]any
			require.NoError(
				t, json.Unmarshal(rec.Body.Bytes(), &resp),
				"response body must be a single well-formed JSON document, not two concatenated: %s", rec.Body.String(),
			)
			assert.Equal(t, "ValidationException", resp["code"])
			assert.Equal(t, "ValidationException: version is required", resp["message"])
		})
	}
}
