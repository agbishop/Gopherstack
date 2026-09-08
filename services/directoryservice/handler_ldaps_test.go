package directoryservice_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/directoryservice"
)

func TestLDAPS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "enable describe disable cycle"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			dirID := mustCreateSimpleAD(t, h, "corp.example.com")

			// Enable
			rec1 := doRequest(t, h, "EnableLDAPS", map[string]any{
				"DirectoryId": dirID,
				"Type":        "Client",
			})
			assert.Equal(t, http.StatusOK, rec1.Code)

			// Describe
			rec2 := doRequest(t, h, "DescribeLDAPSSettings", map[string]any{"DirectoryId": dirID})
			assert.Equal(t, http.StatusOK, rec2.Code)
			var r2 map[string]any
			require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &r2))
			settings, _ := r2["LDAPSSettingsInfo"].([]any)
			require.Len(t, settings, 1)
			setting := settings[0].(map[string]any)
			assert.Equal(t, "Enabled", setting["LDAPSStatus"])

			// Disable
			rec3 := doRequest(t, h, "DisableLDAPS", map[string]any{
				"DirectoryId": dirID,
				"Type":        "Client",
			})
			assert.Equal(t, http.StatusOK, rec3.Code)

			_ = tc
		})
	}
}

// TestLDAPS_InvalidStatus verifies EnableLDAPS/DisableLDAPS reject redundant
// transitions: both ops model InvalidLDAPSStatusException
// (directoryservice@v1.41.4 deserializers.go), whose doc comment reads "The
// LDAP activities could not be performed because they are limited by the
// LDAPS status" (types/errors.go:706-708).
func TestLDAPS_InvalidStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *directoryservice.Handler, dirID string)
		name string
	}{
		{
			name: "EnableLDAPS twice",
			run: func(t *testing.T, h *directoryservice.Handler, dirID string) {
				t.Helper()
				rec1 := doRequest(t, h, "EnableLDAPS", map[string]any{"DirectoryId": dirID, "Type": "Client"})
				require.Equal(t, http.StatusOK, rec1.Code)

				rec2 := doRequest(t, h, "EnableLDAPS", map[string]any{"DirectoryId": dirID, "Type": "Client"})
				assert.Equal(t, http.StatusBadRequest, rec2.Code)
				assert.Equal(t, "InvalidLDAPSStatusException", respBody(t, rec2)["__type"])
			},
		},
		{
			name: "DisableLDAPS without a prior Enable",
			run: func(t *testing.T, h *directoryservice.Handler, dirID string) {
				t.Helper()
				rec := doRequest(t, h, "DisableLDAPS", map[string]any{"DirectoryId": dirID, "Type": "Client"})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Equal(t, "InvalidLDAPSStatusException", respBody(t, rec)["__type"])
			},
		},
		{
			name: "DisableLDAPS twice",
			run: func(t *testing.T, h *directoryservice.Handler, dirID string) {
				t.Helper()
				rec1 := doRequest(t, h, "EnableLDAPS", map[string]any{"DirectoryId": dirID, "Type": "Client"})
				require.Equal(t, http.StatusOK, rec1.Code)
				rec2 := doRequest(t, h, "DisableLDAPS", map[string]any{"DirectoryId": dirID, "Type": "Client"})
				require.Equal(t, http.StatusOK, rec2.Code)

				rec3 := doRequest(t, h, "DisableLDAPS", map[string]any{"DirectoryId": dirID, "Type": "Client"})
				assert.Equal(t, http.StatusBadRequest, rec3.Code)
				assert.Equal(t, "InvalidLDAPSStatusException", respBody(t, rec3)["__type"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			dirID := mustCreateSimpleAD(t, h, "corp.example.com")
			tt.run(t, h, dirID)
		})
	}
}

func TestEnableLDAPS_InvalidType(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	dirID := mustCreateSimpleAD(t, h, "corp.example.com")

	rec := doRequest(t, h, "EnableLDAPS", map[string]any{
		"DirectoryId": dirID,
		"Type":        "Server",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "InvalidParameterException", body["__type"])
}
