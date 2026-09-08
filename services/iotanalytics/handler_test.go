package iotanalytics_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iotanalytics"
)

// newTestHandler creates an in-memory backend + handler for HTTP tests.
func newTestHandler(t *testing.T) *iotanalytics.Handler {
	t.Helper()

	return iotanalytics.NewHandler(iotanalytics.NewInMemoryBackend())
}

// validPipelineActivitiesBody returns a minimal valid pipelineActivities raw-JSON body (one
// channel + one datastore activity), satisfying CreatePipeline/UpdatePipeline's documented
// "must contain both a channel and a datastore activity" requirement
// (api_op_CreatePipeline.go).
func validPipelineActivitiesBody() []map[string]any {
	return []map[string]any{
		{"channel": map[string]any{"name": "ch", "channelName": "src_channel"}},
		{"datastore": map[string]any{"name": "ds", "datastoreName": "sink_datastore"}},
	}
}

// doRequest performs an HTTP request against the handler.
func doRequest(
	t *testing.T,
	h *iotanalytics.Handler,
	method, path string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()

	var bodyBytes []byte

	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)
	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func TestHandler_RouteMatcher(t *testing.T) {
	t.Parallel()

	h := iotanalytics.NewHandler(iotanalytics.NewInMemoryBackend())
	matcher := h.RouteMatcher()

	tests := []struct {
		name    string
		path    string
		service string
		want    bool
	}{
		{
			name:    "channels",
			path:    "/channels",
			service: "iotanalytics",
			want:    true,
		},
		{
			name:    "channels_name",
			path:    "/channels/my-channel",
			service: "iotanalytics",
			want:    true,
		},
		{
			name: "channels_without_iotanalytics_service",
			path: "/channels",
			want: false,
		},
		{
			name: "datastores",
			path: "/datastores",
			want: true,
		},
		{
			name: "datasets",
			path: "/datasets",
			want: true,
		},
		{
			name: "pipelines",
			path: "/pipelines",
			want: true,
		},
		{
			name:    "tags_with_iotanalytics",
			path:    "/tags",
			service: "iotanalytics",
			want:    true,
		},
		{
			name:    "tags_without_service",
			path:    "/tags",
			service: "",
			want:    false,
		},
		{
			name:    "logging_with_iotanalytics",
			path:    "/logging",
			service: "iotanalytics",
			want:    true,
		},
		{
			name:    "logging_without_service",
			path:    "/logging",
			service: "",
			want:    false,
		},
		{
			name:    "messages_with_iotanalytics",
			path:    "/messages/batch",
			service: "iotanalytics",
			want:    true,
		},
		{
			name:    "pipelineactivities_with_iotanalytics",
			path:    "/pipelineactivities/run",
			service: "iotanalytics",
			want:    true,
		},
		{
			name: "other_path",
			path: "/vaults",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)

			if tt.service != "" {
				req.Header.Set(
					"Authorization",
					"AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20240101/us-east-1/"+tt.service+"/aws4_request",
				)
			}

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			got := matcher(c)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestHandler_HandlerReset verifies that Handler.Reset clears backend state reachable
// through the HTTP surface.
func TestHandler_HandlerReset(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/channels", map[string]any{"channelName": "ch1"})
	doRequest(t, h, http.MethodPost, "/channels", map[string]any{"channelName": "ch2"})

	h.Reset()

	rec := doRequest(t, h, http.MethodGet, "/channels", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"channelSummaries":[]`)
}

// TestHandler_ProviderInit_NilAppContext verifies Provider.Init rejects a nil AppContext.
func TestHandler_ProviderInit_NilAppContext(t *testing.T) {
	t.Parallel()

	p := &iotanalytics.Provider{}

	_, err := p.Init(nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, iotanalytics.ErrNilAppContext)
}

// TestHandler_OpsTableSize verifies the pre-built dispatch table has one entry per operation.
func TestHandler_OpsTableSize(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	assert.Equal(t, 34, iotanalytics.HandlerOpsLen(h))
}

// TestHandler_SupportedOperationsMatchOpsTable verifies GetSupportedOperations stays in sync
// with the pre-built dispatch table.
func TestHandler_SupportedOperationsMatchOpsTable(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	ops := h.GetSupportedOperations()

	assert.Len(t, ops, 34)
	assert.Len(t, ops, iotanalytics.HandlerOpsLen(h))
}

// TestHandler_GetSupportedOperations verifies all operations are listed.
func TestHandler_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	h := iotanalytics.NewHandler(iotanalytics.NewInMemoryBackend())
	ops := h.GetSupportedOperations()

	expectedOps := []string{
		"BatchPutMessage",
		"CancelPipelineReprocessing",
		"CreateDatasetContent",
		"DeleteDatasetContent",
		"DescribeLoggingOptions",
		"GetDatasetContent",
		"ListDatasetContents",
		"PutLoggingOptions",
		"RunPipelineActivity",
		"SampleChannelData",
		"StartPipelineReprocessing",
	}

	for _, op := range expectedOps {
		assert.Contains(t, ops, op, "expected operation %s in GetSupportedOperations", op)
	}
}

// TestHandler_NotFound_Returns404 verifies that describing a missing resource of any family
// returns 404, regardless of resource type.
func TestHandler_NotFound_Returns404(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "channel", method: http.MethodGet, path: "/channels/no-such-channel"},
		{name: "datastore", method: http.MethodGet, path: "/datastores/no-such-ds"},
		{name: "dataset", method: http.MethodGet, path: "/datasets/no-such-set"},
		{name: "pipeline", method: http.MethodGet, path: "/pipelines/no-such-pipe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, tt.method, tt.path, nil)
			assert.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}

// TestHandler_AlreadyExistsErrorType verifies ResourceAlreadyExistsException type across
// every resource family.
func TestHandler_AlreadyExistsErrorType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body map[string]any
		name string
		path string
	}{
		{
			name: "channel_conflict",
			path: "/channels",
			body: map[string]any{"channelName": "dup_ch2"},
		},
		{
			name: "datastore_conflict",
			path: "/datastores",
			body: map[string]any{"datastoreName": "dup_ds2"},
		},
		{
			name: "dataset_conflict",
			path: "/datasets",
			body: map[string]any{"datasetName": "dup_set2"},
		},
		{
			name: "pipeline_conflict",
			path: "/pipelines",
			body: map[string]any{
				"pipelineName":       "dup_pipe2",
				"pipelineActivities": validPipelineActivitiesBody(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec1 := doRequest(t, h, http.MethodPost, tt.path, tt.body)
			require.Equal(t, http.StatusOK, rec1.Code)

			rec2 := doRequest(t, h, http.MethodPost, tt.path, tt.body)
			require.Equal(t, http.StatusConflict, rec2.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))
			assert.Equal(t, "ResourceAlreadyExistsException", resp["__type"])
		})
	}
}

// TestHandler_ErrorResponseFormat verifies the AWS-style __type field in error responses.
func TestHandler_ErrorResponseFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		method    string
		path      string
		body      any
		wantType  string
		wantCode  int
		seedFirst bool
	}{
		{
			name:     "not_found_has_ResourceNotFoundException",
			method:   http.MethodGet,
			path:     "/channels/nonexistent",
			wantType: "ResourceNotFoundException",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "datastore_not_found",
			method:   http.MethodGet,
			path:     "/datastores/nonexistent",
			wantType: "ResourceNotFoundException",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "pipeline_not_found",
			method:   http.MethodGet,
			path:     "/pipelines/nonexistent",
			wantType: "ResourceNotFoundException",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "dataset_not_found",
			method:   http.MethodGet,
			path:     "/datasets/nonexistent",
			wantType: "ResourceNotFoundException",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "invalid_name_has_InvalidRequestException",
			method:   http.MethodPost,
			path:     "/channels",
			body:     map[string]any{"channelName": "bad-name"},
			wantType: "InvalidRequestException",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, tt.method, tt.path, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantType, resp["__type"], "error response must have __type field")
			assert.NotEmpty(t, resp["message"], "error response must have message field")
		})
	}
}

// TestHandler_ResourceNameValidation verifies that only [a-zA-Z0-9_]+ names are accepted,
// across every resource family.
func TestHandler_ResourceNameValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		resource   string
		body       map[string]any
		path       string
		wantStatus int
	}{
		{
			name:       "channel_hyphen_rejected",
			path:       "/channels",
			body:       map[string]any{"channelName": "bad-name"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "channel_underscore_accepted",
			path:       "/channels",
			body:       map[string]any{"channelName": "good_name"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "channel_alphanumeric_accepted",
			path:       "/channels",
			body:       map[string]any{"channelName": "GoodName123"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "channel_dot_rejected",
			path:       "/channels",
			body:       map[string]any{"channelName": "bad.name"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "channel_space_rejected",
			path:       "/channels",
			body:       map[string]any{"channelName": "bad name"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "datastore_hyphen_rejected",
			path:       "/datastores",
			body:       map[string]any{"datastoreName": "bad-ds"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "datastore_valid",
			path:       "/datastores",
			body:       map[string]any{"datastoreName": "valid_ds"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "dataset_hyphen_rejected",
			path:       "/datasets",
			body:       map[string]any{"datasetName": "bad-set"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "dataset_valid",
			path:       "/datasets",
			body:       map[string]any{"datasetName": "valid_set"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "pipeline_hyphen_rejected",
			path:       "/pipelines",
			body:       map[string]any{"pipelineName": "bad-pipe"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "pipeline_valid",
			path: "/pipelines",
			body: map[string]any{
				"pipelineName":       "valid_pipe",
				"pipelineActivities": validPipelineActivitiesBody(),
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPost, tt.path, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_Pagination verifies maxResults and nextToken behavior on list endpoints,
// across every resource family.
func TestHandler_Pagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		seedKind   string
		seedCount  int
		maxResults int
		wantLen    int
		wantToken  bool
	}{
		{
			name:       "channels_maxResults_2_of_5",
			seedKind:   "channel",
			seedCount:  5,
			maxResults: 2,
			wantLen:    2,
			wantToken:  true,
		},
		{
			name:       "channels_maxResults_larger_than_count",
			seedKind:   "channel",
			seedCount:  3,
			maxResults: 10,
			wantLen:    3,
			wantToken:  false,
		},
		{
			name:       "datastores_maxResults_1",
			seedKind:   "datastore",
			seedCount:  3,
			maxResults: 1,
			wantLen:    1,
			wantToken:  true,
		},
		{
			name:       "datasets_maxResults_2",
			seedKind:   "dataset",
			seedCount:  4,
			maxResults: 2,
			wantLen:    2,
			wantToken:  true,
		},
		{
			name:       "pipelines_maxResults_3_of_3",
			seedKind:   "pipeline",
			seedCount:  3,
			maxResults: 3,
			wantLen:    3,
			wantToken:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			var listPath string
			var summaryKey string

			for i := range tt.seedCount {
				var body map[string]any
				var createPath string

				switch tt.seedKind {
				case "channel":
					createPath = "/channels"
					listPath = "/channels"
					summaryKey = "channelSummaries"
					body = map[string]any{
						"channelName": strings.ReplaceAll(
							t.Name(),
							"/",
							"_",
						) + "_" + string(
							rune('a'+i),
						),
					}
				case "datastore":
					createPath = "/datastores"
					listPath = "/datastores"
					summaryKey = "datastoreSummaries"
					body = map[string]any{
						"datastoreName": strings.ReplaceAll(
							t.Name(),
							"/",
							"_",
						) + "_" + string(
							rune('a'+i),
						),
					}
				case "dataset":
					createPath = "/datasets"
					listPath = "/datasets"
					summaryKey = "datasetSummaries"
					body = map[string]any{
						"datasetName": strings.ReplaceAll(
							t.Name(),
							"/",
							"_",
						) + "_" + string(
							rune('a'+i),
						),
					}
				case "pipeline":
					createPath = "/pipelines"
					listPath = "/pipelines"
					summaryKey = "pipelineSummaries"
					body = map[string]any{
						"pipelineName": strings.ReplaceAll(
							t.Name(),
							"/",
							"_",
						) + "_" + string(
							rune('a'+i),
						),
						"pipelineActivities": validPipelineActivitiesBody(),
					}
				}

				rec := doRequest(t, h, http.MethodPost, createPath, body)
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doRequest(
				t,
				h,
				http.MethodGet,
				listPath+"?maxResults="+string(rune('0'+tt.maxResults)),
				nil,
			)
			assert.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			summaries, ok := resp[summaryKey].([]any)
			require.True(t, ok)
			assert.Len(t, summaries, tt.wantLen)

			_, hasToken := resp["nextToken"]
			if tt.wantToken {
				assert.True(t, hasToken, "expected nextToken in response")
			} else {
				assert.False(t, hasToken, "expected no nextToken when all results fit")
			}
		})
	}
}

// TestHandler_PaginationNextToken verifies that nextToken retrieves the next page.
func TestHandler_PaginationNextToken(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Seed 3 channels: p2_a, p2_b, p2_c (sorted alphabetically).
	for _, name := range []string{"p2_a", "p2_b", "p2_c"} {
		rec := doRequest(t, h, http.MethodPost, "/channels", map[string]any{"channelName": name})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// Page 1: maxResults=2.
	rec1 := doRequest(t, h, http.MethodGet, "/channels?maxResults=2", nil)
	require.Equal(t, http.StatusOK, rec1.Code)

	var resp1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &resp1))
	summaries1, ok := resp1["channelSummaries"].([]any)
	require.True(t, ok)
	assert.Len(t, summaries1, 2)
	token, ok := resp1["nextToken"].(string)
	require.True(t, ok, "page 1 must have nextToken")
	require.NotEmpty(t, token)

	// Page 2: use the nextToken.
	rec2 := doRequest(t, h, http.MethodGet, "/channels?maxResults=2&nextToken="+token, nil)
	require.Equal(t, http.StatusOK, rec2.Code)

	var resp2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))
	summaries2, ok := resp2["channelSummaries"].([]any)
	require.True(t, ok)
	assert.Len(t, summaries2, 1, "page 2 should have the remaining 1 channel")
	_, hasToken := resp2["nextToken"]
	assert.False(t, hasToken, "page 2 should not have a nextToken")
}

// TestHandler_UpdateHandlersAcceptBody verifies Update* handlers parse request bodies,
// across every resource family.
func TestHandler_UpdateHandlersAcceptBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		createBody map[string]any
		updateBody map[string]any
		name       string
		createPath string
		updatePath string
		wantStatus int
	}{
		{
			name:       "update_channel_with_retention",
			createPath: "/channels",
			createBody: map[string]any{"channelName": "upd_ch"},
			updatePath: "/channels/upd_ch",
			updateBody: map[string]any{
				"retentionPeriod": map[string]any{"numberOfDays": 7},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "update_datastore_with_retention",
			createPath: "/datastores",
			createBody: map[string]any{"datastoreName": "upd_ds"},
			updatePath: "/datastores/upd_ds",
			updateBody: map[string]any{
				"retentionPeriod": map[string]any{"unlimited": true},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "update_dataset_with_versioning",
			createPath: "/datasets",
			createBody: map[string]any{"datasetName": "upd_set"},
			updatePath: "/datasets/upd_set",
			updateBody: map[string]any{
				"versioningConfiguration": map[string]any{"unlimited": true},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "update_pipeline_with_activities",
			createPath: "/pipelines",
			createBody: map[string]any{
				"pipelineName":       "upd_pipe",
				"pipelineActivities": validPipelineActivitiesBody(),
			},
			updatePath: "/pipelines/upd_pipe",
			updateBody: map[string]any{
				"pipelineActivities": []map[string]any{
					{"channel": map[string]any{"name": "ch_act", "channelName": "mych"}},
					{"datastore": map[string]any{"name": "ds_act", "datastoreName": "myds"}},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "update_channel_nil_body_ok",
			createPath: "/channels",
			createBody: map[string]any{"channelName": "upd_ch2"},
			updatePath: "/channels/upd_ch2",
			updateBody: nil,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(t, h, http.MethodPost, tt.createPath, tt.createBody)
			require.Equal(t, http.StatusOK, rec.Code)

			rec = doRequest(t, h, http.MethodPut, tt.updatePath, tt.updateBody)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
