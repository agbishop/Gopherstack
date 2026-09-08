package mediaconvert_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/mediaconvert"
)

func TestMediaConvert_CreateResourceShare_TableTests(t *testing.T) {
	t.Parallel()

	newJob := func(h *mediaconvert.Handler) string {
		rec := doRequest(t, h, http.MethodPost, "/2017-08-29/jobs", map[string]any{
			"role": "arn:aws:iam::123456789012:role/MediaConvert_Role",
		})
		require.Equal(t, http.StatusCreated, rec.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		job, _ := resp["job"].(map[string]any)
		id, _ := job["id"].(string)

		return id
	}

	tests := []struct {
		setup      func(h *mediaconvert.Handler) string
		body       func(jobID string) any
		name       string
		wantStatus int
	}{
		{
			name:  "create_resource_share_valid_job",
			setup: newJob,
			body: func(jobID string) any {
				return map[string]any{"jobId": jobID, "supportCaseId": "case-1234"}
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "create_resource_share_missing_job_id",
			setup:      func(_ *mediaconvert.Handler) string { return "" },
			body:       func(_ string) any { return map[string]any{"description": "no job id"} },
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "create_resource_share_missing_support_case_id",
			setup: newJob,
			body: func(jobID string) any {
				return map[string]any{"jobId": jobID}
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "create_resource_share_job_not_found",
			setup: func(_ *mediaconvert.Handler) string { return "nonexistent-job" },
			body: func(jobID string) any {
				return map[string]any{"jobId": jobID, "supportCaseId": "case-1234"}
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			jobID := tt.setup(h)
			rec := doRequest(t, h, http.MethodPost, "/2017-08-29/resourceShares", tt.body(jobID))
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestCreateResourceShare_EmptyJobID verifies empty jobID returns ErrValidation.
func TestCreateResourceShare_EmptyJobID(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	_, err := b.CreateResourceShare("", "case-1234")
	require.ErrorIs(t, err, mediaconvert.ErrValidation)
}

// TestCreateResourceShare_EmptySupportCaseID verifies empty supportCaseID
// returns ErrValidation. CreateResourceShareInput.SupportCaseId is "This
// member is required" (aws-sdk-go-v2 mediaconvert api_op_CreateResourceShare.go).
func TestCreateResourceShare_EmptySupportCaseID(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	j, err := b.CreateJob("arn:aws:iam::123:role/role", "", "", nil, nil, nil, "")
	require.NoError(t, err)

	_, err = b.CreateResourceShare(j.ID, "")
	require.ErrorIs(t, err, mediaconvert.ErrValidation)
}

// TestCreateResourceShare_SetsShareStatus verifies share sets status.
func TestCreateResourceShare_SetsShareStatus(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	j, err := b.CreateJob("arn:aws:iam::123:role/role", "", "", nil, nil, nil, "")
	require.NoError(t, err)
	require.Equal(t, "NOT_SHARED", j.ShareStatus)

	_, err = b.CreateResourceShare(j.ID, "case-1234")
	require.NoError(t, err)

	got, err := b.GetJob(j.ID)
	require.NoError(t, err)
	assert.Equal(t, "SHARED", got.ShareStatus)
	require.NotNil(t, got.LastShareDetails)

	var details struct {
		ShareToken string  `json:"shareToken"`
		SharedAt   float64 `json:"sharedAt"`
	}
	require.NoError(t, json.Unmarshal([]byte(*got.LastShareDetails), &details))
	assert.NotEmpty(t, details.ShareToken)
	assert.Greater(t, details.SharedAt, float64(0))
}

// TestCreateResourceShare_ViaHTTP sets share on job.
func TestCreateResourceShare_ViaHTTP(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	h := mediaconvert.NewHandler(b)
	j, err := b.CreateJob("arn:aws:iam::123:role/role", "", "", nil, nil, nil, "")
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodPost, "/2017-08-29/resourceShares",
		map[string]any{"jobId": j.ID, "supportCaseId": "case-1234"})
	require.Equal(t, http.StatusNoContent, rec.Code)

	got, err := b.GetJob(j.ID)
	require.NoError(t, err)
	assert.Equal(t, "SHARED", got.ShareStatus)
}
