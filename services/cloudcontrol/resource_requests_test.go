package cloudcontrol_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudcontrol"
)

func TestHandler_GetResourceRequestStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*cloudcontrol.Handler) string
		name       string
		wantOp     string
		wantStatus int
	}{
		{
			name: "success after create",
			setup: func(h *cloudcontrol.Handler) string {
				event, _ := h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"status-test"}`, "")

				return event.RequestToken
			},
			wantStatus: http.StatusOK,
			wantOp:     "CREATE",
		},
		{
			name: "not found returns 400 RequestTokenNotFoundException",
			setup: func(_ *cloudcontrol.Handler) string {
				return "nonexistent-token"
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing RequestToken returns 400",
			setup: func(_ *cloudcontrol.Handler) string {
				return ""
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			token := tt.setup(h)

			body := map[string]any{"RequestToken": token}
			rec := doRequest(t, h, "GetResourceRequestStatus", body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				pe, ok := out["ProgressEvent"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.wantOp, pe["Operation"])
			}
		})
	}
}

// TestHandler_GetResourceRequestStatus_UnknownToken_IsRequestTokenNotFound verifies that an
// unrecognized RequestToken surfaces as RequestTokenNotFoundException -- the ONLY error
// GetResourceRequestStatus declares -- not ResourceNotFoundException, which describes a
// missing *resource*, not a missing *request token*. See the GetResourceRequestStatus
// API reference's Errors section.
func TestHandler_GetResourceRequestStatus_UnknownToken_IsRequestTokenNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "GetResourceRequestStatus", map[string]any{"RequestToken": "nonexistent"})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "RequestTokenNotFoundException", errType(t, rec.Body.Bytes()))
}

// TestHandler_GetResourceRequestStatus_EmptyRequestToken_IsRequestTokenNotFound verifies that
// an empty RequestToken is NOT InvalidRequestException -- GetResourceRequestStatus declares
// only RequestTokenNotFoundException (confirmed against deserializeOpErrorGetResourceRequestStatus
// in the pinned SDK; a real client can never even send an empty RequestToken since it is a
// required member enforced by the SDK's own client-side validators.go, but this backend must
// still answer correctly for a direct wire request that bypasses that client-side check).
func TestHandler_GetResourceRequestStatus_EmptyRequestToken_IsRequestTokenNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "GetResourceRequestStatus", map[string]any{"RequestToken": ""})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	gotErrType := errType(t, rec.Body.Bytes())
	assert.Equal(t, "RequestTokenNotFoundException", gotErrType)
	assert.NotEqual(t, "InvalidRequestException", gotErrType)
}
func TestBackend_GetResourceRequestStatus_EventsNotRemovedOnRead(t *testing.T) {
	t.Parallel()

	b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")

	ev, err := b.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"persist-test"}`, "")
	require.NoError(t, err)

	// First read.
	ev1, err := b.GetResourceRequestStatus(ev.RequestToken)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", ev1.OperationStatus)

	// Second read — event must still be present (not deleted after first read).
	ev2, err := b.GetResourceRequestStatus(ev.RequestToken)
	require.NoError(t, err)
	assert.Equal(t, ev1.RequestToken, ev2.RequestToken)
}
func TestHandler_CancelResourceRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*cloudcontrol.Handler) string
		name         string
		wantOpStatus string
		wantStatus   int
	}{
		{
			name: "success cancels in_progress request",
			setup: func(h *cloudcontrol.Handler) string {
				token := "in-progress-token"
				h.Backend.AddProgressEvent(&cloudcontrol.ProgressEvent{
					RequestToken:    token,
					TypeName:        "AWS::Logs::LogGroup",
					Identifier:      "cancel-test",
					Operation:       "CREATE",
					OperationStatus: "IN_PROGRESS",
				})

				return token
			},
			wantStatus:   http.StatusOK,
			wantOpStatus: "CANCEL_COMPLETE",
		},
		{
			// Real AWS returns ConcurrentModificationException (HTTP 500) for a
			// terminal-status request, not a client validation error -- see the
			// CancelResourceRequest API reference's Errors section.
			name: "cancelling terminal request returns 500 ConcurrentModificationException",
			setup: func(h *cloudcontrol.Handler) string {
				event, _ := h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"terminal-test"}`, "")

				return event.RequestToken
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			// RequestTokenNotFoundException is HTTP 400, the only error this
			// operation declares for an unrecognized token.
			name: "not found returns 400 RequestTokenNotFoundException",
			setup: func(_ *cloudcontrol.Handler) string {
				return "nonexistent-token"
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing RequestToken returns 400",
			setup: func(_ *cloudcontrol.Handler) string {
				return ""
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			token := tt.setup(h)

			body := map[string]any{"RequestToken": token}
			rec := doRequest(t, h, "CancelResourceRequest", body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				pe, ok := out["ProgressEvent"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.wantOpStatus, pe["OperationStatus"])
			}
		})
	}
}

// TestHandler_CancelResourceRequest_OnlyAllowsInProgress verifies that AWS Cloud Control only
// allows cancellation of IN_PROGRESS requests; other statuses (SUCCESS, FAILED,
// CANCEL_COMPLETE, CANCEL_IN_PROGRESS, PENDING) must return HTTP 500
// ConcurrentModificationException, per the CancelResourceRequest API reference's
// Errors section.
func TestHandler_CancelResourceRequest_OnlyAllowsInProgress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   string
		wantHTTP int
	}{
		{
			name:     "in_progress_can_be_cancelled",
			status:   "IN_PROGRESS",
			wantHTTP: http.StatusOK,
		},
		{
			name:     "success_cannot_be_cancelled",
			status:   "SUCCESS",
			wantHTTP: http.StatusInternalServerError,
		},
		{
			name:     "failed_cannot_be_cancelled",
			status:   "FAILED",
			wantHTTP: http.StatusInternalServerError,
		},
		{
			name:     "cancel_complete_cannot_be_cancelled",
			status:   "CANCEL_COMPLETE",
			wantHTTP: http.StatusInternalServerError,
		},
		{
			name:     "cancel_in_progress_cannot_be_cancelled",
			status:   "CANCEL_IN_PROGRESS",
			wantHTTP: http.StatusInternalServerError,
		},
		{
			name:     "pending_cannot_be_cancelled",
			status:   "PENDING",
			wantHTTP: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			token := "test-token-" + tt.name
			h.Backend.AddProgressEvent(&cloudcontrol.ProgressEvent{
				RequestToken:    token,
				TypeName:        "AWS::Logs::LogGroup",
				Identifier:      "test-group",
				Operation:       "CREATE",
				OperationStatus: tt.status,
			})

			rec := doRequest(t, h, "CancelResourceRequest", map[string]any{
				"RequestToken": token,
			})
			assert.Equal(t, tt.wantHTTP, rec.Code, "status %s: unexpected HTTP response", tt.status)

			if tt.wantHTTP == http.StatusOK {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				pe, ok := out["ProgressEvent"].(map[string]any)
				require.True(t, ok, "ProgressEvent must be present")
				assert.Equal(t, "CANCEL_COMPLETE", pe["OperationStatus"])
			}
		})
	}
}

// TestHandler_CancelResourceRequest_UnknownToken_IsRequestTokenNotFound mirrors the above for
// CancelResourceRequest, which declares RequestTokenNotFoundException alongside
// ConcurrentModificationException.
func TestHandler_CancelResourceRequest_UnknownToken_IsRequestTokenNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CancelResourceRequest", map[string]any{"RequestToken": "nonexistent"})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "RequestTokenNotFoundException", errType(t, rec.Body.Bytes()))
}

// TestHandler_CancelResourceRequest_EmptyRequestToken_IsRequestTokenNotFound verifies that an
// empty RequestToken is NOT InvalidRequestException -- CancelResourceRequest declares only
// RequestTokenNotFoundException/ConcurrentModificationException (confirmed against
// deserializeOpErrorCancelResourceRequest in the pinned SDK; a real client can never even
// send an empty RequestToken since it is a required member enforced by the SDK's own
// client-side validators.go, but this backend must still answer correctly for a direct wire
// request that bypasses that client-side check).
func TestHandler_CancelResourceRequest_EmptyRequestToken_IsRequestTokenNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CancelResourceRequest", map[string]any{"RequestToken": ""})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	gotErrType := errType(t, rec.Body.Bytes())
	assert.Equal(t, "RequestTokenNotFoundException", gotErrType)
	assert.NotEqual(t, "InvalidRequestException", gotErrType)
}

// TestHandler_CancelResourceRequest_TerminalStatus_IsConcurrentModification verifies the exact
// wire error code (not just HTTP status) for cancelling a non-cancellable request.
func TestHandler_CancelResourceRequest_TerminalStatus_IsConcurrentModification(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	event, err := h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"terminal"}`, "")
	require.NoError(t, err)

	rec := doRequest(t, h, "CancelResourceRequest", map[string]any{"RequestToken": event.RequestToken})

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "ConcurrentModificationException", errType(t, rec.Body.Bytes()))
}
func TestHandler_EventTimeIsUnixNumber(t *testing.T) {
	t.Parallel()

	// The AWS CloudControl SDK v2 expects EventTime to be a JSON number (Unix epoch seconds),
	// not a string. Each test case creates its own handler and verifies the wire format.
	tests := []struct {
		getRequest func(t *testing.T, h *cloudcontrol.Handler) *httptest.ResponseRecorder
		name       string
	}{
		{
			name: "create_resource_event_time_is_number",
			getRequest: func(t *testing.T, h *cloudcontrol.Handler) *httptest.ResponseRecorder {
				t.Helper()

				return doRequest(t, h, "CreateResource", map[string]any{
					"TypeName":     "AWS::Logs::LogGroup",
					"DesiredState": `{"LogGroupName":"evt-test"}`,
				})
			},
		},
		{
			name: "get_request_status_event_time_is_number",
			getRequest: func(t *testing.T, h *cloudcontrol.Handler) *httptest.ResponseRecorder {
				t.Helper()
				// First create a resource to obtain a request token.
				createRec := doRequest(t, h, "CreateResource", map[string]any{
					"TypeName":     "AWS::Logs::LogGroup",
					"DesiredState": `{"LogGroupName":"evt-status"}`,
				})
				require.Equal(t, http.StatusOK, createRec.Code)

				var createOut map[string]any
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))
				pe, ok := createOut["ProgressEvent"].(map[string]any)
				require.True(t, ok)

				token, tokenOK := pe["RequestToken"].(string)
				require.True(t, tokenOK, "RequestToken should be a string")
				require.NotEmpty(t, token)

				return doRequest(t, h, "GetResourceRequestStatus", map[string]any{
					"RequestToken": token,
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := tt.getRequest(t, h)

			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			pe, ok := out["ProgressEvent"].(map[string]any)
			require.True(t, ok, "ProgressEvent should be present in response")

			// EventTime must be a JSON number (float64 after json.Unmarshal into any).
			eventTime, exists := pe["EventTime"]
			require.True(t, exists, "EventTime must be present in ProgressEvent")
			_, isNumber := eventTime.(float64)
			assert.True(t, isNumber, "EventTime must be a JSON number (Unix epoch), got %T: %v", eventTime, eventTime)
		})
	}
}
func TestHandler_ListResourceRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup         func(*cloudcontrol.Handler)
		body          map[string]any
		name          string
		wantOpInFirst string
		wantCount     int
		wantStatus    int
	}{
		{
			name: "returns all requests when no filter",
			setup: func(h *cloudcontrol.Handler) {
				_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"req-1"}`, "")
				_, _ = h.Backend.CreateResource("AWS::S3::Bucket", `{"BucketName":"req-2"}`, "")
			},
			body:       map[string]any{},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "empty list when no requests",
			setup:      func(_ *cloudcontrol.Handler) {},
			body:       map[string]any{},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "filter by operation CREATE",
			setup: func(h *cloudcontrol.Handler) {
				_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"filter-1"}`, "")
				_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"filter-2"}`, "")
				// Delete creates a DELETE request; two CREATE requests remain
				_, _ = h.Backend.DeleteResource("AWS::Logs::LogGroup", "filter-1", "")
			},
			body: map[string]any{
				"ResourceRequestStatusFilter": map[string]any{
					"Operations": []string{"CREATE"},
				},
			},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name: "filter by operation status SUCCESS",
			setup: func(h *cloudcontrol.Handler) {
				_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"status-1"}`, "")
			},
			body: map[string]any{
				"ResourceRequestStatusFilter": map[string]any{
					"OperationStatuses": []string{"SUCCESS"},
				},
			},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name: "filter by status that does not match returns empty",
			setup: func(h *cloudcontrol.Handler) {
				_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"no-match"}`, "")
			},
			body: map[string]any{
				"ResourceRequestStatusFilter": map[string]any{
					"OperationStatuses": []string{"IN_PROGRESS"},
				},
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "filter by both operation and status",
			setup: func(h *cloudcontrol.Handler) {
				_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"both-1"}`, "")
				_, _ = h.Backend.DeleteResource("AWS::Logs::LogGroup", "both-1", "")
			},
			body: map[string]any{
				"ResourceRequestStatusFilter": map[string]any{
					"Operations":        []string{"DELETE"},
					"OperationStatuses": []string{"SUCCESS"},
				},
			},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "ListResourceRequests", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				summaries, ok := out["ResourceRequestStatusSummaries"].([]any)
				require.True(t, ok, "ResourceRequestStatusSummaries should be an array")
				assert.Len(t, summaries, tt.wantCount)
			}
		})
	}
}

// TestHandler_ListResourceRequests_FilterByOperationStatus verifies the OperationStatuses filter
// on ListResourceRequests returns only matching events.
func TestHandler_ListResourceRequests_FilterByOperationStatus(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	h.Backend.AddProgressEvent(&cloudcontrol.ProgressEvent{
		RequestToken:    "tok-success",
		TypeName:        "AWS::Logs::LogGroup",
		Identifier:      "grp-success",
		Operation:       "CREATE",
		OperationStatus: "SUCCESS",
	})
	h.Backend.AddProgressEvent(&cloudcontrol.ProgressEvent{
		RequestToken:    "tok-inprogress",
		TypeName:        "AWS::Logs::LogGroup",
		Identifier:      "grp-inprogress",
		Operation:       "CREATE",
		OperationStatus: "IN_PROGRESS",
	})
	h.Backend.AddProgressEvent(&cloudcontrol.ProgressEvent{
		RequestToken:    "tok-failed",
		TypeName:        "AWS::Logs::LogGroup",
		Identifier:      "grp-failed",
		Operation:       "CREATE",
		OperationStatus: "FAILED",
	})

	tests := []struct {
		filter  map[string]any
		name    string
		wantLen int
	}{
		{
			name:    "filter_success_only",
			filter:  map[string]any{"OperationStatuses": []string{"SUCCESS"}},
			wantLen: 1,
		},
		{
			name:    "filter_in_progress_only",
			filter:  map[string]any{"OperationStatuses": []string{"IN_PROGRESS"}},
			wantLen: 1,
		},
		{
			name:    "filter_success_and_failed",
			filter:  map[string]any{"OperationStatuses": []string{"SUCCESS", "FAILED"}},
			wantLen: 2,
		},
		{
			name:    "no_filter_returns_all",
			filter:  map[string]any{},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := map[string]any{}
			if len(tt.filter) > 0 {
				body["ResourceRequestStatusFilter"] = tt.filter
			}

			rec := doRequest(t, h, "ListResourceRequests", body)
			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			summaries, _ := out["ResourceRequestStatusSummaries"].([]any)
			assert.Len(t, summaries, tt.wantLen, "filter %v", tt.filter)
		})
	}
}
func TestHandler_ListResourceRequests_ContainsExpectedFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"fields-test"}`, "")

	rec := doRequest(t, h, "ListResourceRequests", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	summaries, ok := out["ResourceRequestStatusSummaries"].([]any)
	require.True(t, ok)
	require.Len(t, summaries, 1)

	summary, ok := summaries[0].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "CREATE", summary["Operation"])
	assert.Equal(t, "SUCCESS", summary["OperationStatus"])
	assert.Equal(t, "AWS::Logs::LogGroup", summary["TypeName"])
	assert.NotEmpty(t, summary["RequestToken"])
	_, isNumber := summary["EventTime"].(float64)
	assert.True(t, isNumber, "EventTime must be a JSON number (Unix epoch)")
}

// TestHandler_ListResourceRequests_EnumValidation verifies that an unrecognized
// Operations/OperationStatuses value in the filter is NOT InvalidRequestException.
// ListResourceRequests declares ZERO errors in the real model (confirmed: botocore's
// service-2.json has an empty "errors" list for this operation, and the pinned SDK's
// deserializeOpErrorListResourceRequests has no named-error case at all, unlike every
// other CloudControl op) -- so an unrecognized value must return 200 with that
// criterion simply never matching any tracked request, not a 400.
func TestHandler_ListResourceRequests_EnumValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body        map[string]any
		name        string
		wantMatches bool
	}{
		{
			name: "unrecognized operation value matches nothing, no error",
			body: map[string]any{
				"ResourceRequestStatusFilter": map[string]any{
					"Operations": []string{"INVALID_OP"},
				},
			},
			wantMatches: false,
		},
		{
			name: "unrecognized status value matches nothing, no error",
			body: map[string]any{
				"ResourceRequestStatusFilter": map[string]any{
					"OperationStatuses": []string{"BOGUS_STATUS"},
				},
			},
			wantMatches: false,
		},
		{
			name: "valid operations and statuses match",
			body: map[string]any{
				"ResourceRequestStatusFilter": map[string]any{
					"Operations":        []string{"CREATE", "DELETE", "UPDATE"},
					"OperationStatuses": []string{"SUCCESS", "FAILED"},
				},
			},
			wantMatches: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, err := h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"enum-test"}`, "")
			require.NoError(t, err)

			rec := doRequest(t, h, "ListResourceRequests", tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			summaries, ok := out["ResourceRequestStatusSummaries"].([]any)

			if tt.wantMatches {
				require.True(t, ok)
				assert.Len(t, summaries, 1)
			} else {
				assert.Empty(t, summaries)
			}
		})
	}
}

// TestHandler_ListResourceRequests_TypeNameFilterIsIgnored verifies that a
// "TypeName" key inside ResourceRequestStatusFilter has NO filtering effect.
// The real SDK's types.ResourceRequestStatusFilter has exactly two members --
// Operations and OperationStatuses -- confirmed against both
// aws-sdk-go-v2/service/cloudcontrol/types and botocore's service-2.json; it
// has no TypeName member at all. A prior gopherstack pass invented one and
// filtered on it, which is a wire-shape bug: any caller sending TypeName here
// (matching what the real, TypeName-less shape would silently drop) must see
// unfiltered results, not narrower ones.
func TestHandler_ListResourceRequests_TypeNameFilterIsIgnored(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"lg-1"}`, "")
	_, _ = h.Backend.CreateResource("AWS::S3::Bucket", `{"BucketName":"b-1"}`, "")

	rec := doRequest(t, h, "ListResourceRequests", map[string]any{
		"ResourceRequestStatusFilter": map[string]any{
			"TypeName": "AWS::Logs::LogGroup",
		},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	summaries, ok := out["ResourceRequestStatusSummaries"].([]any)
	require.True(t, ok)
	assert.Len(t, summaries, 2, "TypeName has no filtering effect on the real API -- both requests must be returned")
}
func TestHandler_ListResourceRequests_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 4 {
		name := "pg-" + strings.Repeat("r", i+1)
		_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"`+name+`"}`, "")
	}

	rec := doRequest(t, h, "ListResourceRequests", map[string]any{
		"MaxResults": 2,
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	summaries, ok := out["ResourceRequestStatusSummaries"].([]any)
	require.True(t, ok)
	assert.Len(t, summaries, 2)

	nextToken, hasToken := out["NextToken"].(string)
	require.True(t, hasToken, "NextToken should be present")
	require.NotEmpty(t, nextToken)

	rec2 := doRequest(t, h, "ListResourceRequests", map[string]any{
		"MaxResults": 2,
		"NextToken":  nextToken,
	})

	require.Equal(t, http.StatusOK, rec2.Code)

	var out2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out2))

	summaries2, ok := out2["ResourceRequestStatusSummaries"].([]any)
	require.True(t, ok)
	assert.Len(t, summaries2, 2)
}
