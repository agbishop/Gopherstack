package pinpoint_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateTemplate_RejectedUpdate_DoesNotDoubleWrite is a regression test
// for gopherstack-246v (the gopherstack-8haq shape found in pinpoint):
// applyTemplateUpdate rejects an update to a template that does not exist by
// calling writeNotFoundOrInternal -> writeErrorResponse, which writes the 404
// and returns nil (writeErrorResponse always returns nil after a successful
// write). handleUpdateTemplate stored that nil in updateErr and only
// continued past it on updateErr != nil, so the write-then-nil rejection was
// silently accepted as success and a second, corrupting 202 Accepted
// response was written onto the already-committed 404.
//
// Status code alone does not distinguish fixed from broken here: echo's own
// Response.WriteHeader guards on Committed and silently drops the second
// status, so rec.Code is 404 either way. The httptest.ResponseRecorder used
// by doPinpointRequest has no Content-Length enforcement (unlike a real
// net/http server), so the second write's bytes land in rec.Body verbatim --
// that is the observable difference this test checks, the same way
// TestHandler_DescribeCacheClusters_MaxRecordsOutOfRange_DoesNotDoubleWrite
// checks it for elasticache's instance of the same bug.
func TestUpdateTemplate_RejectedUpdate_DoesNotDoubleWrite(t *testing.T) {
	t.Parallel()

	h := newHandlerForTest(t)

	rec := doPinpointRequest(t, h, http.MethodPut,
		"/v1/templates/does-not-exist/email", map[string]any{"Subject": "New Subject"})

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.NotContains(t, rec.Body.String(), "Accepted",
		"a rejected update must not write a second (success) response onto the same body")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp),
		"response body must decode as exactly one JSON object -- a second "+
			"write appended after the committed 404 corrupts it")
	assert.Equal(t, "NotFoundException", resp["__type"])
}
