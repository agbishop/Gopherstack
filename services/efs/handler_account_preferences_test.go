package efs_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPutAccountPreferences_InvalidResourceIdType verifies an unrecognized
// ResourceIdType emits BadRequest, not ValidationException -- PutAccountPreferences
// declares BadRequest/InternalServerError only (efs@v1.44.4 deserializers.go).
func TestPutAccountPreferences_InvalidResourceIdType(t *testing.T) {
	t.Parallel()

	h := newTestEFSHandler()

	rec := doREST(t, h, http.MethodPut, "/2015-02-01/account-preferences", map[string]any{
		"ResourceIdPreference": map[string]any{"ResourceIdType": "BOGUS"},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	resp := parseResp(t, rec)
	assert.Equal(t, "BadRequest", resp["ErrorCode"])
}

// TestDescribeAccountPreferences exercises DescribeAccountPreferences.
func TestDescribeAccountPreferences(t *testing.T) {
	t.Parallel()

	h := newTestEFSHandler()

	rec := doREST(t, h, http.MethodGet, "/2015-02-01/account-preferences", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	pref, ok := resp["ResourceIdPreference"].(map[string]any)
	require.True(t, ok)
	assert.NotEmpty(t, pref["ResourceIdType"])
	resources := pref["Resources"].([]any)
	assert.NotEmpty(t, resources)
}
