package codepipeline_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codepipeline"
)

func TestHandler_AcknowledgeJob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *codepipeline.Handler)
		input      any
		name       string
		wantStatus string
		httpStatus int
		wantErr    bool
	}{
		{
			name: "success_inprogress",
			setup: func(h *codepipeline.Handler) {
				h.Backend.AddJobInternal(&codepipeline.Job{ID: "job-1", Nonce: "nonce-abc", Status: "Created"})
			},
			input:      map[string]any{"jobId": "job-1", "nonce": "nonce-abc"},
			httpStatus: http.StatusOK,
			wantStatus: "InProgress",
		},
		{
			name: "nonce_mismatch_status_unchanged",
			setup: func(h *codepipeline.Handler) {
				h.Backend.AddJobInternal(&codepipeline.Job{ID: "job-2", Nonce: "nonce-right", Status: "Created"})
			},
			input:      map[string]any{"jobId": "job-2", "nonce": "nonce-wrong"},
			httpStatus: http.StatusOK,
			wantStatus: "Created",
		},
		{
			name:       "not_found",
			setup:      nil,
			input:      map[string]any{"jobId": "no-such-job", "nonce": "nonce-1"},
			httpStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "missing_jobId",
			setup:      nil,
			input:      map[string]any{"nonce": "nonce-1"},
			httpStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "missing_nonce",
			setup:      nil,
			input:      map[string]any{"jobId": "job-x"},
			httpStatus: http.StatusBadRequest,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "AcknowledgeJob", tt.input)
			assert.Equal(t, tt.httpStatus, rec.Code)

			if !tt.wantErr {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.Equal(t, tt.wantStatus, out["status"])
			}
		})
	}
}

func TestHandler_GetJobDetails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *codepipeline.Handler)
		input      any
		name       string
		httpStatus int
		wantErr    bool
	}{
		{
			name: "success",
			setup: func(h *codepipeline.Handler) {
				h.Backend.AddJobInternal(&codepipeline.Job{ID: "jd-1", Nonce: "n", Status: "Created"})
			},
			input:      map[string]any{"jobId": "jd-1"},
			httpStatus: http.StatusOK,
		},
		{
			name:       "not_found",
			setup:      nil,
			input:      map[string]any{"jobId": "missing"},
			httpStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "missing_jobId",
			setup:      nil,
			input:      map[string]any{},
			httpStatus: http.StatusBadRequest,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "GetJobDetails", tt.input)
			assert.Equal(t, tt.httpStatus, rec.Code)

			if !tt.wantErr {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				details, ok := out["jobDetails"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "jd-1", details["id"])
			}
		})
	}
}

func TestGetJobDetails_DataPopulated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		category string
		provider string
		version  string
	}{
		{name: "build_action", category: "Build", provider: "MyBuild", version: "1"},
		{name: "deploy_action", category: "Deploy", provider: "MyDeploy", version: "2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			h.Backend.AddJobInternal(&codepipeline.Job{
				ID:    "job-" + tt.name,
				Nonce: "nonce-1",
				ActionTypeID: codepipeline.ActionTypeID{
					Category: tt.category,
					Owner:    "Custom",
					Provider: tt.provider,
					Version:  tt.version,
				},
				Status: "Queued",
			})

			rec := doRequest(t, h, "GetJobDetails", map[string]any{"jobId": "job-" + tt.name})
			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

			details, ok := out["jobDetails"].(map[string]any)
			require.True(t, ok, "jobDetails must be present")

			data, ok := details["data"].(map[string]any)
			require.True(t, ok, "data must be present")

			atID, ok := data["actionTypeId"].(map[string]any)
			require.True(t, ok, "data.actionTypeId must be present")
			assert.Equal(t, tt.category, atID["category"])
			assert.Equal(t, tt.provider, atID["provider"])
			assert.Equal(t, tt.version, atID["version"])
		})
	}
}

// TestParity_PutApprovalResult_ApprovedAt verifies approvedAt is non-empty.

func TestHandler_PollForJobs_Filter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		actionTypeID map[string]any
		setup        func(h *codepipeline.Handler)
		name         string
		wantJobIDs   []string
		wantStatus   int
	}{
		{
			name: "matching category/provider/version returns job",
			setup: func(h *codepipeline.Handler) {
				h.Backend.AddJobInternal(&codepipeline.Job{
					ID:     "job-match-1",
					Nonce:  "n1",
					Status: "Queued",
					ActionTypeID: codepipeline.ActionTypeID{
						Category: "Build", Owner: "Custom", Provider: "MyBuilder", Version: "1",
					},
				})
				h.Backend.AddJobInternal(&codepipeline.Job{
					ID:     "job-other-provider",
					Nonce:  "n2",
					Status: "Queued",
					ActionTypeID: codepipeline.ActionTypeID{
						Category: "Build", Owner: "Custom", Provider: "OtherBuilder", Version: "1",
					},
				})
			},
			actionTypeID: map[string]any{
				"category": "Build", "owner": "Custom", "provider": "MyBuilder", "version": "1",
			},
			wantStatus: http.StatusOK,
			wantJobIDs: []string{"job-match-1"},
		},
		{
			name: "no queued jobs for type returns empty",
			setup: func(h *codepipeline.Handler) {
				h.Backend.AddJobInternal(&codepipeline.Job{
					ID:     "job-inprogress",
					Nonce:  "n1",
					Status: "InProgress",
					ActionTypeID: codepipeline.ActionTypeID{
						Category: "Build", Owner: "Custom", Provider: "MyBuilder", Version: "1",
					},
				})
			},
			actionTypeID: map[string]any{
				"category": "Build", "owner": "Custom", "provider": "MyBuilder", "version": "1",
			},
			wantStatus: http.StatusOK,
			wantJobIDs: []string{},
		},
		{
			name: "different version not returned",
			setup: func(h *codepipeline.Handler) {
				h.Backend.AddJobInternal(&codepipeline.Job{
					ID:     "job-v2",
					Nonce:  "n1",
					Status: "Queued",
					ActionTypeID: codepipeline.ActionTypeID{
						Category: "Build", Owner: "Custom", Provider: "MyBuilder", Version: "2",
					},
				})
			},
			actionTypeID: map[string]any{
				"category": "Build", "owner": "Custom", "provider": "MyBuilder", "version": "1",
			},
			wantStatus: http.StatusOK,
			wantJobIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "PollForJobs", map[string]any{
				"actionTypeId": tt.actionTypeID,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			jobs, _ := out["jobs"].([]any)

			gotIDs := make([]string, len(jobs))
			for i, j := range jobs {
				jm, _ := j.(map[string]any)
				gotIDs[i] = jm["id"].(string)

				// PollForJobs' real Job type carries accountId and a nested
				// data.actionTypeId (types.Job{AccountId, Data, Id, Nonce}) so
				// a job worker can tell what kind of job it received.
				assert.NotEmpty(t, jm["accountId"], "accountId must be present on every polled job")
				data, ok := jm["data"].(map[string]any)
				require.True(t, ok, "data must be present on every polled job")
				actionTypeID, ok := data["actionTypeId"].(map[string]any)
				require.True(t, ok, "data.actionTypeId must be present on every polled job")
				assert.Equal(t, tt.actionTypeID["category"], actionTypeID["category"])
				assert.Equal(t, tt.actionTypeID["provider"], actionTypeID["provider"])
			}

			assert.ElementsMatch(t, tt.wantJobIDs, gotIDs)
		})
	}
}

// --------------------------------------------------------------------------
// #15 & #16 Owner persisted; ListActionTypes full shape
// --------------------------------------------------------------------------

func TestHandler_PollForJobs_MaxBatchSize(t *testing.T) {
	t.Parallel()

	makeJob := func(id string) *codepipeline.Job {
		return &codepipeline.Job{
			ID:     id,
			Nonce:  "n-" + id,
			Status: "Queued",
			ActionTypeID: codepipeline.ActionTypeID{
				Category: "Build",
				Owner:    "Custom",
				Provider: "MyBuilder",
				Version:  "1",
			},
		}
	}

	tests := []struct {
		name         string
		maxBatchSize int32
		wantCount    int
	}{
		{
			name:         "maxBatchSize=1 limits to 1 job",
			maxBatchSize: 1,
			wantCount:    1,
		},
		{
			name:         "maxBatchSize=2 limits to 2 jobs",
			maxBatchSize: 2,
			wantCount:    2,
		},
		{
			name:         "maxBatchSize=0 defaults to at most 10",
			maxBatchSize: 0,
			wantCount:    3,
		},
		{
			name:         "maxBatchSize exceeding count returns all",
			maxBatchSize: 10,
			wantCount:    3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := codepipeline.NewInMemoryBackend("000000000000", "us-east-1")
			b.AddJobInternal(makeJob("job-a"))
			b.AddJobInternal(makeJob("job-b"))
			b.AddJobInternal(makeJob("job-c"))

			h := codepipeline.NewHandler(b)
			rec := doRequest(t, h, "PollForJobs", map[string]any{
				"actionTypeId": map[string]any{
					"category": "Build",
					"owner":    "Custom",
					"provider": "MyBuilder",
					"version":  "1",
				},
				"maxBatchSize": tt.maxBatchSize,
			})
			assert.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

			jobs, _ := out["jobs"].([]any)
			assert.Len(t, jobs, tt.wantCount)
		})
	}
}

func TestHandler_PutJobResult_UpdatesStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *codepipeline.Handler)
		input      map[string]any
		action     string
		name       string
		wantStatus int
	}{
		{
			name:   "success result accepted",
			action: "PutJobSuccessResult",
			setup: func(h *codepipeline.Handler) {
				h.Backend.AddJobInternal(&codepipeline.Job{
					ID: "job-success", Nonce: "n", Status: "InProgress",
					ActionTypeID: codepipeline.ActionTypeID{Category: "Build", Provider: "P", Version: "1"},
				})
			},
			input:      map[string]any{"jobId": "job-success"},
			wantStatus: http.StatusOK,
		},
		{
			name:   "failure result accepted",
			action: "PutJobFailureResult",
			setup: func(h *codepipeline.Handler) {
				h.Backend.AddJobInternal(&codepipeline.Job{
					ID: "job-fail", Nonce: "n", Status: "InProgress",
					ActionTypeID: codepipeline.ActionTypeID{Category: "Build", Provider: "P", Version: "1"},
				})
			},
			input: map[string]any{
				"jobId":          "job-fail",
				"failureDetails": map[string]any{"message": "build failed"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "success result for missing job returns error",
			action:     "PutJobSuccessResult",
			setup:      nil,
			input:      map[string]any{"jobId": "ghost-job"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, tt.action, tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// --------------------------------------------------------------------------
// ArtifactStores (cross-region) round-trip
// --------------------------------------------------------------------------

func TestHandler_JobOperations(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	cat := &codepipeline.CustomActionType{
		Category: "Build",
		Provider: "MyBuild",
		Version:  "1",
	}

	_, err := h.Backend.CreateCustomActionType(context.Background(), cat)
	require.NoError(t, err)

	job := &codepipeline.Job{
		ID:       "job-001",
		Nonce:    "nonce-001",
		ClientID: "token",
	}
	h.Backend.AddJobInternal(job)

	// Poll for jobs
	rec := doRequest(t, h, "PollForJobs", map[string]any{
		"actionTypeId": map[string]any{
			"category": "Build",
			"owner":    "Custom",
			"provider": "MyBuild",
			"version":  "1",
		},
	})
	require.Equal(t, 200, rec.Code)

	// Poll with empty action type id (no validation - returns 200)
	rec = doRequest(t, h, "PollForJobs", map[string]any{})
	assert.Equal(t, 200, rec.Code)

	// Acknowledge job
	rec = doRequest(t, h, "AcknowledgeJob", map[string]any{
		"jobId": "job-001",
		"nonce": "nonce-001",
	})
	assert.Equal(t, 200, rec.Code)

	// Acknowledge with missing jobId
	rec = doRequest(t, h, "AcknowledgeJob", map[string]any{})
	assert.Equal(t, 400, rec.Code)

	// Put job success
	rec = doRequest(t, h, "PutJobSuccessResult", map[string]any{
		"jobId": "job-001",
	})
	assert.Equal(t, 200, rec.Code)

	// Put job success with missing id
	rec = doRequest(t, h, "PutJobSuccessResult", map[string]any{})
	assert.Equal(t, 400, rec.Code)

	// Add another job and test failure result
	h.Backend.AddJobInternal(&codepipeline.Job{
		ID:    "job-002",
		Nonce: "nonce-002",
	})

	rec = doRequest(t, h, "PutJobFailureResult", map[string]any{
		"jobId": "job-002",
		"failureDetails": map[string]any{
			"message": "build failed",
			"type":    "JobFailed",
		},
	})
	assert.Equal(t, 200, rec.Code)

	// Missing jobId
	rec = doRequest(t, h, "PutJobFailureResult", map[string]any{})
	assert.Equal(t, 400, rec.Code)

	// Poll for third party jobs
	rec = doRequest(t, h, "PollForThirdPartyJobs", map[string]any{
		"actionTypeId": map[string]any{
			"category": "Build",
			"owner":    "Custom",
			"provider": "MyBuild",
			"version":  "1",
		},
	})
	assert.Equal(t, 200, rec.Code)

	// Poll third party with empty action type id (no validation - returns 200)
	rec = doRequest(t, h, "PollForThirdPartyJobs", map[string]any{})
	assert.Equal(t, 200, rec.Code)

	// Get third party job details
	rec = doRequest(t, h, "GetThirdPartyJobDetails", map[string]any{
		"jobId":       "job-001",
		"clientToken": "token",
	})
	assert.Equal(t, 200, rec.Code)

	// Get third party with missing jobId
	rec = doRequest(t, h, "GetThirdPartyJobDetails", map[string]any{})
	assert.Equal(t, 400, rec.Code)
}

// ---- Third Party Job tests ----
