package directoryservice_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRadius(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "enable update disable cycle"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			dirID := mustCreateSimpleAD(t, h, "corp.example.com")

			radiusSettings := map[string]any{
				"RadiusServers":   []any{"10.0.0.1"},
				"RadiusPort":      1812,
				"RadiusRetries":   3,
				"RadiusTimeout":   30,
				"SharedSecret":    "secret",
				"UseSameUsername": false,
			}

			// Enable
			rec1 := doRequest(t, h, "EnableRadius", map[string]any{
				"DirectoryId":    dirID,
				"RadiusSettings": radiusSettings,
			})
			assert.Equal(t, http.StatusOK, rec1.Code)

			// Update
			rec2 := doRequest(t, h, "UpdateRadius", map[string]any{
				"DirectoryId":    dirID,
				"RadiusSettings": radiusSettings,
			})
			assert.Equal(t, http.StatusOK, rec2.Code)

			// Disable
			rec3 := doRequest(t, h, "DisableRadius", map[string]any{"DirectoryId": dirID})
			assert.Equal(t, http.StatusOK, rec3.Code)

			_ = tc
		})
	}
}

// TestEnableRadius_AlreadyEnabled verifies that a second EnableRadius call on
// a directory that already has RADIUS enabled returns
// EntityAlreadyExistsException: EnableRadius's own deserializer models
// EntityAlreadyExistsException while DisableRadius/UpdateRadius don't
// (directoryservice@v1.41.4 deserializers.go), and UpdateRadius exists
// specifically to change settings on an already-enabled directory.
func TestEnableRadius_AlreadyEnabled(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	dirID := mustCreateSimpleAD(t, h, "corp.example.com")

	radiusSettings := map[string]any{
		"RadiusServers": []any{"10.0.0.1"},
		"RadiusPort":    1812,
		"SharedSecret":  "secret",
	}

	rec1 := doRequest(t, h, "EnableRadius", map[string]any{
		"DirectoryId":    dirID,
		"RadiusSettings": radiusSettings,
	})
	require.Equal(t, http.StatusOK, rec1.Code)

	rec2 := doRequest(t, h, "EnableRadius", map[string]any{
		"DirectoryId":    dirID,
		"RadiusSettings": radiusSettings,
	})
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
	assert.Equal(t, "EntityAlreadyExistsException", respBody(t, rec2)["__type"])
}
