package accessanalyzer_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/accessanalyzer"
)

// TestPolicyGenerationLifecycle verifies Start/Get/Cancel/List policy generation.
func TestPolicyGenerationLifecycle(t *testing.T) {
	t.Parallel()

	b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
	h := accessanalyzer.NewHandler(b)

	tests := []struct {
		runFn   func() *httptest.ResponseRecorder
		checkFn func(t *testing.T, rec *httptest.ResponseRecorder)
		name    string
	}{
		{
			name: "start_policy_generation",
			runFn: func() *httptest.ResponseRecorder {
				return doRequest(t, h, http.MethodPut, "/policy/generation", map[string]any{
					"policyGenerationDetails": map[string]any{"principalArn": "arn:aws:iam::000000000000:role/MyRole"},
				})
			},
			checkFn: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				assert.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["jobId"])
			},
		},
		{
			name: "list_policy_generations",
			runFn: func() *httptest.ResponseRecorder {
				// start one first
				doRequest(t, h, http.MethodPut, "/policy/generation", map[string]any{
					"policyGenerationDetails": map[string]any{"principalArn": "arn:aws:iam::000000000000:role/R"},
				})

				return doRequest(t, h, http.MethodGet, "/policy/generation", nil)
			},
			checkFn: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				assert.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["policyGenerations"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := tt.runFn()
			tt.checkFn(t, rec)
		})
	}
}

// TestGetGeneratedPolicy verifies getting a generated policy by jobId.
func TestGetGeneratedPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
		missingJob bool
	}{
		{name: "existing_job", wantStatus: http.StatusOK},
		// GetGeneratedPolicy's own deserializeOpError switch
		// (aws-sdk-go-v2/service/accessanalyzer@v1.51.4 deserializers.go) does
		// not type ResourceNotFoundException, so an unrecognized jobId is
		// reported as ValidationException/400, not 404.
		{name: "missing_job", wantStatus: http.StatusBadRequest, missingJob: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
			h := accessanalyzer.NewHandler(b)

			jobID := "missing-job-id"

			if !tt.missingJob {
				rec := doRequest(t, h, http.MethodPut, "/policy/generation", map[string]any{
					"policyGenerationDetails": map[string]any{"principalArn": "arn:aws:iam::000000000000:role/R"},
				})
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				jobID = resp["jobId"]
			}

			rec := doRequest(t, h, http.MethodGet, "/policy/generation/"+jobID, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus != http.StatusOK {
				return
			}

			// types.JobDetails (GetGeneratedPolicyOutput.jobDetails) has no
			// principalArn member -- that value only appears under
			// generatedPolicyResult.properties.principalArn for this operation.
			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			jobDetails, ok := resp["jobDetails"].(map[string]any)
			require.True(t, ok)
			_, hasPrincipalArn := jobDetails["principalArn"]
			assert.False(t, hasPrincipalArn, "jobDetails must not carry principalArn")

			result, ok := resp["generatedPolicyResult"].(map[string]any)
			require.True(t, ok)
			properties, ok := result["properties"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "arn:aws:iam::000000000000:role/R", properties["principalArn"])
		})
	}
}

// TestCancelPolicyGeneration verifies PUT /policy/generation/{jobId} cancels job.
func TestCancelPolicyGeneration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
		missingJob bool
	}{
		{name: "cancel_existing", wantStatus: http.StatusOK},
		// CancelPolicyGeneration's own deserializeOpError switch
		// (aws-sdk-go-v2/service/accessanalyzer@v1.51.4 deserializers.go) does
		// not type ResourceNotFoundException, so an unrecognized jobId is
		// reported as ValidationException/400, not 404.
		{name: "cancel_missing", wantStatus: http.StatusBadRequest, missingJob: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
			h := accessanalyzer.NewHandler(b)

			jobID := "no-such-job"

			if !tt.missingJob {
				rec := doRequest(t, h, http.MethodPut, "/policy/generation", map[string]any{
					"policyGenerationDetails": map[string]any{"principalArn": "arn:aws:iam::000000000000:role/R"},
				})
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				jobID = resp["jobId"]
			}

			rec := doRequest(t, h, http.MethodPut, "/policy/generation/"+jobID, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestStartPolicyGeneration_CloudTrailDetails verifies StartPolicyGenerationInput's
// optional cloudTrailDetails (aws-sdk-go-v2 api_op_StartPolicyGeneration.go,
// v1.51.4) is stored and echoed back by GetGeneratedPolicy as
// generatedPolicyResult.properties.cloudTrailProperties (types.CloudTrailProperties,
// types/types.go:2375) -- previously silently discarded on the floor.
func TestStartPolicyGeneration_CloudTrailDetails(t *testing.T) {
	t.Parallel()

	b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
	h := accessanalyzer.NewHandler(b)

	startRec := doRequest(t, h, http.MethodPut, "/policy/generation", map[string]any{
		"policyGenerationDetails": map[string]any{"principalArn": "arn:aws:iam::000000000000:role/R"},
		"cloudTrailDetails": map[string]any{
			"accessRole": "arn:aws:iam::000000000000:role/access",
			"startTime":  "2026-01-01T00:00:00Z",
			"endTime":    "2026-02-01T00:00:00Z",
			"trails": []map[string]any{
				{"cloudTrailArn": "arn:aws:cloudtrail:us-east-1:000000000000:trail/MyTrail", "allRegions": true},
			},
		},
	})
	require.Equal(t, http.StatusOK, startRec.Code)

	var startResp map[string]string
	require.NoError(t, json.Unmarshal(startRec.Body.Bytes(), &startResp))

	getRec := doRequest(t, h, http.MethodGet, "/policy/generation/"+startResp["jobId"], nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &resp))

	result, ok := resp["generatedPolicyResult"].(map[string]any)
	require.True(t, ok)
	properties, ok := result["properties"].(map[string]any)
	require.True(t, ok)

	ctp, ok := properties["cloudTrailProperties"].(map[string]any)
	require.True(t, ok, "cloudTrailProperties must be echoed back when cloudTrailDetails was supplied")
	assert.Equal(t, "2026-01-01T00:00:00Z", ctp["startTime"])
	assert.Equal(t, "2026-02-01T00:00:00Z", ctp["endTime"])

	trails, ok := ctp["trailProperties"].([]any)
	require.True(t, ok)
	require.Len(t, trails, 1)

	trail, ok := trails[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "arn:aws:cloudtrail:us-east-1:000000000000:trail/MyTrail", trail["cloudTrailArn"])
	assert.Equal(t, true, trail["allRegions"])

	_, hasAccessRole := ctp["accessRole"]
	assert.False(t, hasAccessRole, "accessRole has no CloudTrailProperties counterpart")
}

// TestListPolicyGenerations_MaxResultsAndNextToken verifies
// ListPolicyGenerationsInput's maxResults/nextToken (real wire query
// params -- serializers.go:2571-2577 in aws-sdk-go-v2/service/accessanalyzer
// @v1.51.4, both guarded by `!= nil`) are honored for pagination, instead of
// always returning every job in one page with no nextToken.
func TestListPolicyGenerations_MaxResultsAndNextToken(t *testing.T) {
	t.Parallel()

	b := accessanalyzer.NewInMemoryBackend("000000000000", "us-east-1")
	h := accessanalyzer.NewHandler(b)

	for range 3 {
		_, err := b.StartPolicyGeneration("arn:aws:iam::000000000000:role/R", nil)
		require.NoError(t, err)
	}

	firstRec := doRequest(t, h, http.MethodGet, "/policy/generation?maxResults=1", nil)
	require.Equal(t, http.StatusOK, firstRec.Code)

	var firstResp map[string]any
	require.NoError(t, json.Unmarshal(firstRec.Body.Bytes(), &firstResp))

	firstPage, ok := firstResp["policyGenerations"].([]any)
	require.True(t, ok)
	require.Len(t, firstPage, 1, "maxResults=1 must truncate the page to one job")

	nextToken, ok := firstResp["nextToken"].(string)
	require.True(t, ok && nextToken != "", "a truncated page must carry a nextToken")

	secondRec := doRequest(t, h, http.MethodGet, "/policy/generation?maxResults=1&nextToken="+nextToken, nil)
	require.Equal(t, http.StatusOK, secondRec.Code)

	var secondResp map[string]any
	require.NoError(t, json.Unmarshal(secondRec.Body.Bytes(), &secondResp))

	secondPage, ok := secondResp["policyGenerations"].([]any)
	require.True(t, ok)
	require.Len(t, secondPage, 1)

	first, ok := firstPage[0].(map[string]any)
	require.True(t, ok)
	second, ok := secondPage[0].(map[string]any)
	require.True(t, ok)
	assert.NotEqual(t, first["jobId"], second["jobId"], "the second page must not repeat the first job")
}
