package codepipeline_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codepipeline"
)

func TestHandler_AcknowledgeThirdPartyJob(t *testing.T) {
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
			name: "success",
			setup: func(h *codepipeline.Handler) {
				h.Backend.AddJobInternal(&codepipeline.Job{
					ID: "tp-job-1", Nonce: "tp-nonce", Status: "Created", ClientID: "token-abc",
				})
			},
			input: map[string]any{
				"jobId":       "tp-job-1",
				"nonce":       "tp-nonce",
				"clientToken": "token-abc",
			},
			httpStatus: http.StatusOK,
			wantStatus: "InProgress",
		},
		{
			name:       "missing_clientToken",
			setup:      nil,
			input:      map[string]any{"jobId": "x", "nonce": "y"},
			httpStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "not_found",
			setup:      nil,
			input:      map[string]any{"jobId": "no-such", "nonce": "n", "clientToken": "t"},
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

			rec := doRequest(t, h, "AcknowledgeThirdPartyJob", tt.input)
			assert.Equal(t, tt.httpStatus, rec.Code)

			if !tt.wantErr {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.Equal(t, tt.wantStatus, out["status"])
			}
		})
	}
}

func TestHandler_ThirdPartyJobResults(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	h.Backend.AddJobInternal(&codepipeline.Job{
		ID:       "tp-job-001",
		Nonce:    "nonce-tp-001",
		ClientID: "token",
	})

	// Acknowledge third party job
	rec := doRequest(t, h, "AcknowledgeThirdPartyJob", map[string]any{
		"jobId":       "tp-job-001",
		"nonce":       "nonce-tp-001",
		"clientToken": "token",
	})
	assert.Equal(t, 200, rec.Code)

	// Missing jobId
	rec = doRequest(t, h, "AcknowledgeThirdPartyJob", map[string]any{})
	assert.Equal(t, 400, rec.Code)

	// Put third party job success
	rec = doRequest(t, h, "PutThirdPartyJobSuccessResult", map[string]any{
		"jobId":       "tp-job-001",
		"clientToken": "token",
	})
	assert.Equal(t, 200, rec.Code)

	// Missing jobId
	rec = doRequest(t, h, "PutThirdPartyJobSuccessResult", map[string]any{})
	assert.Equal(t, 400, rec.Code)

	h.Backend.AddJobInternal(&codepipeline.Job{
		ID:       "tp-job-002",
		Nonce:    "nonce-tp-002",
		ClientID: "token",
	})

	// Put third party job failure
	rec = doRequest(t, h, "PutThirdPartyJobFailureResult", map[string]any{
		"jobId":       "tp-job-002",
		"clientToken": "token",
		"failureDetails": map[string]any{
			"message": "failed",
			"type":    "JobFailed",
		},
	})
	assert.Equal(t, 200, rec.Code)

	// Missing jobId
	rec = doRequest(t, h, "PutThirdPartyJobFailureResult", map[string]any{})
	assert.Equal(t, 400, rec.Code)
}

// TestHandler_PollForThirdPartyJobs_WireShape proves PollForThirdPartyJobs
// returns AWS's real ThirdPartyJob shape ({jobId}, no "nonce" -- a
// third-party worker only learns the nonce later via
// GetThirdPartyJobDetails) rather than the plain-Job-shaped {id, nonce} this
// handler previously (and wrongly) returned.
func TestHandler_PollForThirdPartyJobs_WireShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "queued third-party job"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			h.Backend.AddJobInternal(&codepipeline.Job{
				ID:     "tp-job-shape",
				Nonce:  "should-not-leak",
				Status: "Queued",
				ActionTypeID: codepipeline.ActionTypeID{
					Category: "Build", Owner: "ThirdParty", Provider: "MyBuild", Version: "1",
				},
			})

			rec := doRequest(t, h, "PollForThirdPartyJobs", map[string]any{
				"actionTypeId": map[string]any{
					"category": "Build", "owner": "ThirdParty", "provider": "MyBuild", "version": "1",
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			jobs, _ := out["jobs"].([]any)
			require.Len(t, jobs, 1)

			job, _ := jobs[0].(map[string]any)
			assert.Equal(t, "tp-job-shape", job["jobId"], "the real ThirdPartyJob field is jobId, not id")
			assert.NotContains(t, job, "id", "id is not a real ThirdPartyJob member")
			assert.NotContains(t, job, "nonce", "nonce must not leak before GetThirdPartyJobDetails")
		})
	}
}

// TestGetThirdPartyJobDetails_DataPopulated mirrors
// TestGetJobDetails_DataPopulated: ThirdPartyJobDetails.Data.ActionTypeId is
// a real member that must round-trip.
func TestGetThirdPartyJobDetails_DataPopulated(t *testing.T) {
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
				ID:    "tp-job-" + tt.name,
				Nonce: "nonce-1",
				ActionTypeID: codepipeline.ActionTypeID{
					Category: tt.category,
					Owner:    "ThirdParty",
					Provider: tt.provider,
					Version:  tt.version,
				},
				Status:   "Queued",
				ClientID: "token",
			})

			rec := doRequest(t, h, "GetThirdPartyJobDetails", map[string]any{
				"jobId": "tp-job-" + tt.name, "clientToken": "token",
			})
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

// ---- Action revision and approval ----
