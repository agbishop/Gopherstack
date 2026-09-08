package iotanalytics_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_CreateAndDescribePipeline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pipelineName string
		wantStatus   int
	}{
		{
			name:         "success",
			pipelineName: "test_pipeline",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "empty_name",
			pipelineName: "",
			wantStatus:   http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPost, "/pipelines", map[string]any{
				"pipelineName":       tt.pipelineName,
				"pipelineActivities": validPipelineActivitiesBody(),
			})

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				rec2 := doRequest(t, h, http.MethodGet, "/pipelines/"+tt.pipelineName, nil)
				assert.Equal(t, http.StatusOK, rec2.Code)
			}
		})
	}
}

func TestHandler_StartAndCancelPipelineReprocessing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pipelineName string
		seed         bool
		wantStart    int
		wantCancel   int
	}{
		{
			name:         "success",
			pipelineName: "my_pipeline",
			seed:         true,
			wantStart:    http.StatusCreated,
			wantCancel:   http.StatusNoContent,
		},
		{
			name:         "pipeline_not_found",
			pipelineName: "nonexistent",
			seed:         false,
			wantStart:    http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.seed {
				rec := doRequest(
					t,
					h,
					http.MethodPost,
					"/pipelines",
					map[string]any{
						"pipelineName":       tt.pipelineName,
						"pipelineActivities": validPipelineActivitiesBody(),
					},
				)
				require.Equal(t, http.StatusOK, rec.Code)
			}

			startRec := doRequest(
				t,
				h,
				http.MethodPost,
				"/pipelines/"+tt.pipelineName+"/reprocessing",
				nil,
			)
			assert.Equal(t, tt.wantStart, startRec.Code)

			if tt.wantStart == http.StatusCreated {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(startRec.Body.Bytes(), &resp))
				reprocessingID, ok := resp["reprocessingId"].(string)
				require.True(t, ok)
				assert.NotEmpty(t, reprocessingID)

				cancelRec := doRequest(
					t,
					h,
					http.MethodDelete,
					"/pipelines/"+tt.pipelineName+"/reprocessing/"+reprocessingID,
					nil,
				)
				assert.Equal(t, tt.wantCancel, cancelRec.Code)
			}
		})
	}
}

func TestHandler_RunPipelineActivity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       any
		name       string
		wantStatus int
		wantLen    int
	}{
		{
			name: "with_payloads",
			body: map[string]any{
				"pipelineActivity": map[string]any{
					"channel": map[string]any{"name": "ch", "channelName": "test"},
				},
				"payloads": [][]byte{[]byte(`{"val":1}`), []byte(`{"val":2}`)},
			},
			wantStatus: http.StatusOK,
			wantLen:    2,
		},
		{
			name: "empty_payloads",
			body: map[string]any{
				"pipelineActivity": map[string]any{},
				"payloads":         [][]byte{},
			},
			wantStatus: http.StatusOK,
			wantLen:    0,
		},
		{
			name:       "invalid_body",
			body:       "not-json",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPost, "/pipelineactivities/run", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				payloads, _ := resp["payloads"].([]any)
				assert.Len(t, payloads, tt.wantLen)
			}
		})
	}
}

// TestHandler_Pipelines covers pipeline CRUD operations.
func TestHandler_Pipelines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pipelineName string
		op           string
		wantStatus   int
	}{
		{
			name:         "list",
			pipelineName: "list_pipeline",
			op:           "list",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "update_success",
			pipelineName: "update_pipeline",
			op:           "update",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "delete_success",
			pipelineName: "delete_pipeline",
			op:           "delete",
			wantStatus:   http.StatusNoContent,
		},
		{
			name:         "update_not_found",
			pipelineName: "no_such_pipeline",
			op:           "update_only",
			wantStatus:   http.StatusNotFound,
		},
		{
			name:         "delete_not_found",
			pipelineName: "no_such_pipeline",
			op:           "delete_only",
			wantStatus:   http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			switch tt.op {
			case "list":
				doRequest(t, h, http.MethodPost, "/pipelines", map[string]any{
					"pipelineName": tt.pipelineName, "pipelineActivities": validPipelineActivitiesBody(),
				})
				listRec := doRequest(t, h, http.MethodGet, "/pipelines", nil)
				assert.Equal(t, tt.wantStatus, listRec.Code)
				var resp map[string]any
				require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &resp))
				summaries, ok := resp["pipelineSummaries"].([]any)
				require.True(t, ok)
				assert.Len(t, summaries, 1)

			case "update":
				doRequest(t, h, http.MethodPost, "/pipelines", map[string]any{
					"pipelineName": tt.pipelineName, "pipelineActivities": validPipelineActivitiesBody(),
				})
				rec := doRequest(t, h, http.MethodPut, "/pipelines/"+tt.pipelineName, nil)
				assert.Equal(t, tt.wantStatus, rec.Code)

			case "delete":
				doRequest(t, h, http.MethodPost, "/pipelines", map[string]any{
					"pipelineName": tt.pipelineName, "pipelineActivities": validPipelineActivitiesBody(),
				})
				rec := doRequest(t, h, http.MethodDelete, "/pipelines/"+tt.pipelineName, nil)
				assert.Equal(t, tt.wantStatus, rec.Code)

			case "update_only":
				rec := doRequest(t, h, http.MethodPut, "/pipelines/"+tt.pipelineName, nil)
				assert.Equal(t, tt.wantStatus, rec.Code)

			case "delete_only":
				rec := doRequest(t, h, http.MethodDelete, "/pipelines/"+tt.pipelineName, nil)
				assert.Equal(t, tt.wantStatus, rec.Code)
			}
		})
	}
}

// TestHandler_ErrAlreadyExists_Pipeline verifies creating a duplicate pipeline returns 409.
func TestHandler_ErrAlreadyExists_Pipeline(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/pipelines", map[string]any{
		"pipelineName": "dup_pipe", "pipelineActivities": validPipelineActivitiesBody(),
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doRequest(t, h, http.MethodPost, "/pipelines", map[string]any{
		"pipelineName": "dup_pipe", "pipelineActivities": validPipelineActivitiesBody(),
	})
	assert.Equal(t, http.StatusConflict, rec2.Code)
}

// TestHandler_CancelPipelineReprocessing_NotFound tests cancel with invalid reprocessing ID.
func TestHandler_CancelPipelineReprocessing_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	pipelineName := "cancel_test_pipeline"

	// Create pipeline.
	rec := doRequest(t, h, http.MethodPost, "/pipelines", map[string]any{
		"pipelineName": pipelineName, "pipelineActivities": validPipelineActivitiesBody(),
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Cancel with non-existent reprocessing ID.
	cancelRec := doRequest(
		t,
		h,
		http.MethodDelete,
		"/pipelines/"+pipelineName+"/reprocessing/nonexistent-id",
		nil,
	)
	assert.Equal(t, http.StatusNotFound, cancelRec.Code)
}

// TestHandler_DescribePipeline_NewFields verifies pipeline describe response fields.
func TestHandler_DescribePipeline_NewFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pipelineName string
		body         map[string]any
		wantFields   []string
	}{
		{
			name:         "basic_has_pipeline_wrapper",
			pipelineName: "desc_pipe1",
			body: map[string]any{
				"pipelineName":       "desc_pipe1",
				"pipelineActivities": validPipelineActivitiesBody(),
			},
			wantFields: []string{"pipeline"},
		},
		{
			name:         "with_activities",
			pipelineName: "desc_pipe2",
			body: map[string]any{
				"pipelineName": "desc_pipe2",
				"pipelineActivities": []map[string]any{
					{"channel": map[string]any{"name": "ch_activity", "channelName": "src"}},
					{"datastore": map[string]any{"name": "ds_activity", "datastoreName": "sink"}},
				},
			},
			wantFields: []string{"pipeline"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(t, h, http.MethodPost, "/pipelines", tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			descRec := doRequest(t, h, http.MethodGet, "/pipelines/"+tt.pipelineName, nil)
			require.Equal(t, http.StatusOK, descRec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))
			for _, field := range tt.wantFields {
				assert.Contains(t, resp, field, "describe response must contain field %q", field)
			}
		})
	}
}

// TestHandler_StartPipelineReprocessing_TimeRange verifies StartTime/EndTime validation.
func TestHandler_StartPipelineReprocessing_TimeRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       any
		name       string
		wantStatus int
	}{
		{
			name:       "no_times_ok",
			body:       map[string]any{},
			wantStatus: http.StatusCreated,
		},
		{
			name: "valid_time_range",
			body: map[string]any{
				"startTime": 1000.0,
				"endTime":   2000.0,
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "start_after_end_rejected",
			body: map[string]any{
				"startTime": 3000.0,
				"endTime":   1000.0,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "start_equals_end_rejected",
			body: map[string]any{
				"startTime": 1000.0,
				"endTime":   1000.0,
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(
				t,
				h,
				http.MethodPost,
				"/pipelines",
				map[string]any{
					"pipelineName":       "rp_pipe",
					"pipelineActivities": validPipelineActivitiesBody(),
				},
			)
			require.Equal(t, http.StatusOK, rec.Code)

			startRec := doRequest(t, h, http.MethodPost, "/pipelines/rp_pipe/reprocessing", tt.body)
			assert.Equal(t, tt.wantStatus, startRec.Code)
		})
	}
}

// TestHandler_ReprocessingSummaries_Sorted verifies reprocessing summaries are typed and sorted.
func TestHandler_ReprocessingSummaries_Sorted(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(
		t,
		h,
		http.MethodPost,
		"/pipelines",
		map[string]any{
			"pipelineName":       "rps_pipe",
			"pipelineActivities": validPipelineActivitiesBody(),
		},
	)
	require.Equal(t, http.StatusOK, rec.Code)

	// Start 3 reprocessings.
	ids := make([]string, 3)
	for i := range ids {
		startRec := doRequest(
			t,
			h,
			http.MethodPost,
			"/pipelines/rps_pipe/reprocessing",
			map[string]any{},
		)
		require.Equal(t, http.StatusCreated, startRec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(startRec.Body.Bytes(), &resp))
		ids[i], _ = resp["reprocessingId"].(string)
		require.NotEmpty(t, ids[i])
	}

	// DescribePipeline should list all reprocessings in the response.
	descRec := doRequest(t, h, http.MethodGet, "/pipelines/rps_pipe", nil)
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))
	pipeline, ok := resp["pipeline"].(map[string]any)
	require.True(t, ok)
	summaries, ok := pipeline["reprocessingSummaries"].([]any)
	require.True(t, ok)
	assert.Len(t, summaries, 3)

	// Each summary should have id and status fields.
	for _, s := range summaries {
		summary, isMap := s.(map[string]any)
		require.True(t, isMap)
		assert.NotEmpty(t, summary["id"])
		assert.NotEmpty(t, summary["status"])
	}
}

// TestHandler_PipelineReprocessingSummary_StartTime verifies StartTime appears in reprocessing summaries.
func TestHandler_PipelineReprocessingSummary_StartTime(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(
		t,
		h,
		http.MethodPost,
		"/pipelines",
		map[string]any{
			"pipelineName":       "rp_start_pl",
			"pipelineActivities": validPipelineActivitiesBody(),
		},
	)

	body := map[string]any{
		"startTime": 1000.0,
		"endTime":   2000.0,
	}
	startRec := doRequest(t, h, http.MethodPost, "/pipelines/rp_start_pl/reprocessing", body)
	require.Equal(t, http.StatusCreated, startRec.Code)

	descRec := doRequest(t, h, http.MethodGet, "/pipelines/rp_start_pl", nil)
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))
	pl, _ := resp["pipeline"].(map[string]any)
	summaries, _ := pl["reprocessingSummaries"].([]any)
	require.Len(t, summaries, 1)
	summary, _ := summaries[0].(map[string]any)
	assert.NotZero(t, summary["startTime"], "startTime must appear in reprocessing summary")
}

// TestHandler_RunPipelineActivity_PayloadLimit verifies RunPipelineActivity rejects more than 10 payloads.
func TestHandler_RunPipelineActivity_PayloadLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		count      int
		wantStatus int
	}{
		{
			name:       "10_payloads_ok",
			count:      10,
			wantStatus: http.StatusOK,
		},
		{
			name:       "11_payloads_rejected",
			count:      11,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "1_payload_ok",
			count:      1,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			payloads := make([][]byte, tt.count)

			for i := range payloads {
				payloads[i] = []byte(`{"x":1}`)
			}

			body := map[string]any{
				"pipelineActivity": map[string]any{},
				"payloads":         payloads,
			}
			rec := doRequest(t, h, http.MethodPost, "/pipelineactivities/run", body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
