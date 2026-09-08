package directoryservice_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/directoryservice"
)

func TestClientAuthentication(t *testing.T) {
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
			rec1 := doRequest(t, h, "EnableClientAuthentication", map[string]any{
				"DirectoryId": dirID,
				"Type":        "SmartCard",
			})
			assert.Equal(t, http.StatusOK, rec1.Code)

			// Describe
			rec2 := doRequest(t, h, "DescribeClientAuthenticationSettings", map[string]any{"DirectoryId": dirID})
			assert.Equal(t, http.StatusOK, rec2.Code)
			var r2 map[string]any
			require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &r2))
			settings, _ := r2["ClientAuthenticationSettingsInfo"].([]any)
			require.Len(t, settings, 1)
			setting := settings[0].(map[string]any)
			assert.Equal(t, "Enabled", setting["Status"])

			// Disable
			rec3 := doRequest(t, h, "DisableClientAuthentication", map[string]any{
				"DirectoryId": dirID,
				"Type":        "SmartCard",
			})
			assert.Equal(t, http.StatusOK, rec3.Code)

			_ = tc
		})
	}
}

// TestClientAuthentication_InvalidStatus verifies Enable/DisableClientAuthentication
// reject redundant transitions: both ops model InvalidClientAuthStatusException,
// whose doc comment reads "Client authentication is already enabled."
// (directoryservice@v1.41.4 types/errors.go:678-679).
func TestClientAuthentication_InvalidStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *directoryservice.Handler, dirID string)
		name string
	}{
		{
			name: "EnableClientAuthentication twice",
			run: func(t *testing.T, h *directoryservice.Handler, dirID string) {
				t.Helper()
				rec1 := doRequest(t, h, "EnableClientAuthentication",
					map[string]any{"DirectoryId": dirID, "Type": "SmartCard"})
				require.Equal(t, http.StatusOK, rec1.Code)

				rec2 := doRequest(t, h, "EnableClientAuthentication",
					map[string]any{"DirectoryId": dirID, "Type": "SmartCard"})
				assert.Equal(t, http.StatusBadRequest, rec2.Code)
				assert.Equal(t, "InvalidClientAuthStatusException", respBody(t, rec2)["__type"])
			},
		},
		{
			name: "DisableClientAuthentication without a prior Enable",
			run: func(t *testing.T, h *directoryservice.Handler, dirID string) {
				t.Helper()
				rec := doRequest(t, h, "DisableClientAuthentication",
					map[string]any{"DirectoryId": dirID, "Type": "SmartCard"})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Equal(t, "InvalidClientAuthStatusException", respBody(t, rec)["__type"])
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

func TestEnableClientAuthentication_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		reqType string
		name    string
	}{
		{name: "invalid Type returns InvalidParameterException", reqType: "Fingerprint"},
		{name: "missing Type returns InvalidParameterException", reqType: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			dirID := mustCreateSimpleAD(t, h, "corp.example.com")

			rec := doRequest(t, h, "EnableClientAuthentication", map[string]any{
				"DirectoryId": dirID,
				"Type":        tt.reqType,
			})
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			var body map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "InvalidParameterException", body["__type"])
		})
	}
}
