package macie2_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/macie2"
)

// TestUpdateClassificationScope_ExcludedBucketsSurviveIndependentAdds guards
// gopherstack-c8ge: types.S3ClassificationScopeExclusionUpdate carries an
// explicit ADD/REMOVE/REPLACE Operation discriminator rather than accepting
// a full replacement list. Adding bucket B in a later call must not drop
// bucket A, added by an earlier, unrelated ADD call; a later REMOVE must
// only take out the bucket it names.
func TestUpdateClassificationScope_ExcludedBucketsSurviveIndependentAdds(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodGet, "/classification-scopes", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	scopes, _ := listResp["classificationScopes"].([]any)
	require.Len(t, scopes, 1)
	scope0, _ := scopes[0].(map[string]any)
	scopeID, _ := scope0["id"].(string)
	require.NotEmpty(t, scopeID)

	// Update A: ADD bucket-a.
	rec = doRequest(t, h, http.MethodPatch, "/classification-scopes/"+scopeID, map[string]any{
		"s3": map[string]any{
			"excludes": map[string]any{
				"bucketNames": []any{"bucket-a"},
				"operation":   "ADD",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Update B: ADD bucket-b, without mentioning bucket-a.
	rec = doRequest(t, h, http.MethodPatch, "/classification-scopes/"+scopeID, map[string]any{
		"s3": map[string]any{
			"excludes": map[string]any{
				"bucketNames": []any{"bucket-b"},
				"operation":   "ADD",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/classification-scopes/"+scopeID, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	s3, ok := got["s3"].(map[string]any)
	require.True(t, ok, "got %#v", got)
	excludes, ok := s3["excludes"].(map[string]any)
	require.True(t, ok, "got %#v", s3)
	assert.ElementsMatch(t, []any{"bucket-a", "bucket-b"}, excludes["bucketNames"],
		"bucket-a must survive an ADD that never mentioned it")

	// Update C: REMOVE bucket-a only.
	rec = doRequest(t, h, http.MethodPatch, "/classification-scopes/"+scopeID, map[string]any{
		"s3": map[string]any{
			"excludes": map[string]any{
				"bucketNames": []any{"bucket-a"},
				"operation":   "REMOVE",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/classification-scopes/"+scopeID, nil)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	s3, ok = got["s3"].(map[string]any)
	require.True(t, ok)
	excludes, ok = s3["excludes"].(map[string]any)
	require.True(t, ok)
	assert.ElementsMatch(t, []any{"bucket-b"}, excludes["bucketNames"],
		"REMOVE must only take out the bucket it names")
}

func TestClassificationJobs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fn   func(t *testing.T, h *macie2.Handler)
		name string
	}{
		{
			name: "create_describe_list_update",
			fn: func(t *testing.T, h *macie2.Handler) {
				t.Helper()

				// CreateClassificationJob
				rec := doRequest(t, h, http.MethodPost, "/jobs", map[string]any{
					"name":    "test-job",
					"jobType": "ONE_TIME",
					"s3JobDefinition": map[string]any{
						"bucketDefinitions": []any{},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				var createResp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
				jobID := createResp["jobId"]
				assert.NotEmpty(t, jobID)
				assert.Equal(t, "RUNNING", createResp["jobStatus"])
				// Real CreateClassificationJobOutput always includes jobArn.
				assert.Contains(t, createResp["jobArn"], "arn:aws:macie2:")

				// DescribeClassificationJob
				rec = doRequest(t, h, http.MethodGet, "/jobs/"+jobID, nil)
				assert.Equal(t, http.StatusOK, rec.Code)

				var descResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
				assert.Equal(t, jobID, descResp["jobId"])
				assert.Equal(t, "ONE_TIME", descResp["jobType"])
				assert.Equal(t, "test-job", descResp["name"])
				assert.Equal(t, createResp["jobArn"], descResp["jobArn"])
				assert.NotEmpty(t, descResp["lastRunTime"])

				// ListClassificationJobs
				rec = doRequest(t, h, http.MethodPost, "/jobs/list", nil)
				assert.Equal(t, http.StatusOK, rec.Code)

				var listResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
				items, _ := listResp["items"].([]any)
				require.Len(t, items, 1)
				item0 := items[0].(map[string]any)
				assert.NotEmpty(t, item0["lastRunTime"])

				// UpdateClassificationJob
				rec = doRequest(t, h, http.MethodPatch, "/jobs/"+jobID, map[string]any{
					"jobStatus": "CANCELLED",
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				// Verify update
				rec = doRequest(t, h, http.MethodGet, "/jobs/"+jobID, nil)
				var updated map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
				assert.Equal(t, "CANCELLED", updated["jobStatus"])
			},
		},
		{
			name: "describe_missing_returns_404",
			fn: func(t *testing.T, h *macie2.Handler) {
				t.Helper()

				rec := doRequest(t, h, http.MethodGet, "/jobs/nonexistent", nil)
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "scheduled_job_starts_idle",
			fn: func(t *testing.T, h *macie2.Handler) {
				t.Helper()

				rec := doRequest(t, h, http.MethodPost, "/jobs", map[string]any{
					"name":    "sched-job",
					"jobType": "SCHEDULED",
					"scheduleFrequency": map[string]any{
						"dailySchedule": map[string]any{},
					},
				})
				require.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				rec = doRequest(t, h, http.MethodGet, "/jobs/"+resp["jobId"], nil)
				var desc map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &desc))
				assert.Equal(t, "IDLE", desc["jobStatus"])
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.fn(t, newTestHandler(t))
		})
	}
}

func TestClassificationConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fn   func(t *testing.T, h *macie2.Handler)
		name string
	}{
		{
			name: "get_put_classification_export_config",
			fn: func(t *testing.T, h *macie2.Handler) {
				t.Helper()

				// GetClassificationExportConfiguration — empty by default
				rec := doRequest(t, h, http.MethodGet, "/classification-export-configuration", nil)
				assert.Equal(t, http.StatusOK, rec.Code)

				var getResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
				cfg, _ := getResp["configuration"].(map[string]any)
				assert.Nil(t, cfg["s3Destination"])

				// PutClassificationExportConfiguration
				rec = doRequest(t, h, http.MethodPut, "/classification-export-configuration", map[string]any{
					"configuration": map[string]any{
						"s3Destination": map[string]any{
							"bucketName": "my-export-bucket",
							"keyPrefix":  "macie/",
							"kmsKeyArn":  "arn:aws:kms:us-east-1:000000000000:key/abc",
						},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				// Verify
				rec = doRequest(t, h, http.MethodGet, "/classification-export-configuration", nil)
				var updated map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
				updCfg, _ := updated["configuration"].(map[string]any)
				s3Dest, _ := updCfg["s3Destination"].(map[string]any)
				assert.Equal(t, "my-export-bucket", s3Dest["bucketName"])
			},
		},
		{
			name: "list_and_get_and_update_classification_scope",
			fn: func(t *testing.T, h *macie2.Handler) {
				t.Helper()

				// ListClassificationScopes — auto-creates default
				rec := doRequest(t, h, http.MethodGet, "/classification-scopes", nil)
				assert.Equal(t, http.StatusOK, rec.Code)

				var listResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
				scopes, _ := listResp["classificationScopes"].([]any)
				require.Len(t, scopes, 1)

				scope0, _ := scopes[0].(map[string]any)
				scopeID, _ := scope0["id"].(string)
				require.NotEmpty(t, scopeID)

				// GetClassificationScope
				rec = doRequest(t, h, http.MethodGet, "/classification-scopes/"+scopeID, nil)
				assert.Equal(t, http.StatusOK, rec.Code)

				var scopeResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &scopeResp))
				assert.Equal(t, scopeID, scopeResp["id"])

				// UpdateClassificationScope
				rec = doRequest(t, h, http.MethodPatch, "/classification-scopes/"+scopeID, map[string]any{
					"s3": map[string]any{
						"excludes": map[string]any{
							"bucketNames": []any{"bucket1"},
							"operation":   "ADD",
						},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "get_missing_scope_returns_404",
			fn: func(t *testing.T, h *macie2.Handler) {
				t.Helper()

				rec := doRequest(t, h, http.MethodGet, "/classification-scopes/nonexistent", nil)
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.fn(t, newTestHandler(t))
		})
	}
}

// createTestJob creates a classification job of the given name/jobType and
// returns (jobId, jobArn).
func createTestJob(t *testing.T, h *macie2.Handler, name, jobType string) (string, string) {
	t.Helper()

	body := map[string]any{
		"name":    name,
		"jobType": jobType,
		"s3JobDefinition": map[string]any{
			"bucketDefinitions": []any{},
		},
	}
	if jobType == "SCHEDULED" {
		body["scheduleFrequency"] = map[string]any{"dailySchedule": map[string]any{}}
	}

	rec := doRequest(t, h, http.MethodPost, "/jobs", body)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	return resp["jobId"], resp["jobArn"]
}

// TestListClassificationJobsFilterAndPagination locks the ListClassificationJobs
// gap fix: filterCriteria (includes/excludes, EQ/NE) and maxResults/nextToken
// must actually filter and page results instead of always returning every job
// in one page.
func TestListClassificationJobsFilterAndPagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	oneTimeID, _ := createTestJob(t, h, "job-one-time", "ONE_TIME")
	_, _ = createTestJob(t, h, "job-scheduled", "SCHEDULED")

	t.Run("includes EQ jobType filters to matching jobs", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodPost, "/jobs/list", map[string]any{
			"filterCriteria": map[string]any{
				"includes": []any{
					map[string]any{"key": "jobType", "comparator": "EQ", "values": []any{"ONE_TIME"}},
				},
			},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		items, ok := resp["items"].([]any)
		require.True(t, ok)
		require.Len(t, items, 1)

		item, ok := items[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, oneTimeID, item["jobId"])
	})

	t.Run("excludes NE jobStatus excludes matching jobs", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodPost, "/jobs/list", map[string]any{
			"filterCriteria": map[string]any{
				"excludes": []any{
					map[string]any{"key": "jobStatus", "comparator": "NE", "values": []any{"IDLE"}},
				},
			},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		items, ok := resp["items"].([]any)
		require.True(t, ok)

		// excludes: jobStatus NE IDLE means "exclude anything whose status
		// isn't IDLE" -- only the SCHEDULED (IDLE) job should survive.
		require.Len(t, items, 1)
		item, ok := items[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "IDLE", item["jobStatus"])
	})

	t.Run("maxResults paginates and nextToken advances", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodPost, "/jobs/list", map[string]any{"maxResults": 1})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		items, ok := resp["items"].([]any)
		require.True(t, ok)
		require.Len(t, items, 1)
		require.NotEmpty(t, resp["nextToken"])

		rec2 := doRequest(t, h, http.MethodPost, "/jobs/list", map[string]any{
			"maxResults": 1,
			"nextToken":  resp["nextToken"],
		})
		require.Equal(t, http.StatusOK, rec2.Code)

		var resp2 map[string]any
		require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))
		items2, ok := resp2["items"].([]any)
		require.True(t, ok)
		require.Len(t, items2, 1)
		assert.Empty(t, resp2["nextToken"])
	})
}

// TestClassificationJobUserPausedDetails locks that UpdateClassificationJob
// populates userPausedDetails only while jobStatus is USER_PAUSED, and clears
// it again on any other transition (real DescribeClassificationJobOutput
// only carries userPausedDetails in that one state).
func TestClassificationJobUserPausedDetails(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	jobID, _ := createTestJob(t, h, "pausable-job", "ONE_TIME")

	rec := doRequest(t, h, http.MethodPatch, "/jobs/"+jobID, map[string]any{"jobStatus": "USER_PAUSED"})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/jobs/"+jobID, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var paused map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &paused))
	details, ok := paused["userPausedDetails"].(map[string]any)
	require.True(t, ok, "userPausedDetails must be present while USER_PAUSED")
	assert.NotEmpty(t, details["jobPausedAt"])
	assert.NotEmpty(t, details["jobExpiresAt"])

	rec = doRequest(t, h, http.MethodPatch, "/jobs/"+jobID, map[string]any{"jobStatus": "RUNNING"})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/jobs/"+jobID, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resumed map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resumed))
	assert.Nil(t, resumed["userPausedDetails"], "userPausedDetails must clear once no longer USER_PAUSED")
}

// TestUpdateClassificationJob_InvalidTransitions locks
// UpdateClassificationJobInput.JobStatus's doc comment
// (api_op_UpdateClassificationJob.go:37-58): CANCELLED is valid only from
// IDLE/PAUSED/RUNNING/USER_PAUSED, RUNNING only from USER_PAUSED, and
// USER_PAUSED only from IDLE/PAUSED/RUNNING. A target outside that
// three-value set isn't a documented "Valid value" for the field at all.
func TestUpdateClassificationJob_InvalidTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *macie2.Handler) string
		name       string
		targetStat string
		wantCode   int
	}{
		{
			name: "RUNNING target requires USER_PAUSED, not RUNNING itself",
			setup: func(t *testing.T, h *macie2.Handler) string {
				t.Helper()
				jobID, _ := createTestJob(t, h, "already-running", "ONE_TIME")

				return jobID
			},
			targetStat: "RUNNING",
			wantCode:   http.StatusConflict,
		},
		{
			name: "USER_PAUSED target rejected from CANCELLED",
			setup: func(t *testing.T, h *macie2.Handler) string {
				t.Helper()
				jobID, _ := createTestJob(t, h, "cancel-then-pause", "ONE_TIME")
				rec := doRequest(t, h, http.MethodPatch, "/jobs/"+jobID, map[string]any{"jobStatus": "CANCELLED"})
				require.Equal(t, http.StatusOK, rec.Code)

				return jobID
			},
			targetStat: "USER_PAUSED",
			wantCode:   http.StatusConflict,
		},
		{
			name: "CANCELLED target rejected once already CANCELLED",
			setup: func(t *testing.T, h *macie2.Handler) string {
				t.Helper()
				jobID, _ := createTestJob(t, h, "double-cancel", "ONE_TIME")
				rec := doRequest(t, h, http.MethodPatch, "/jobs/"+jobID, map[string]any{"jobStatus": "CANCELLED"})
				require.Equal(t, http.StatusOK, rec.Code)

				return jobID
			},
			targetStat: "CANCELLED",
			wantCode:   http.StatusConflict,
		},
		{
			name: "unrecognized target status rejected",
			setup: func(t *testing.T, h *macie2.Handler) string {
				t.Helper()
				jobID, _ := createTestJob(t, h, "bad-target", "ONE_TIME")

				return jobID
			},
			targetStat: "COMPLETE",
			wantCode:   http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			jobID := tt.setup(t, h)

			rec := doRequest(t, h, http.MethodPatch, "/jobs/"+jobID, map[string]any{"jobStatus": tt.targetStat})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestClassificationJobTagging locks the tags gap fix: TagResource/
// ListTagsForResource/UntagResource must recognize classification-job ARNs
// (isKnownARN), not just allow-list/custom-data-identifier/findings-filter.
func TestClassificationJobTagging(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, jobArn := createTestJob(t, h, "taggable-job", "ONE_TIME")

	rec := doRequest(t, h, http.MethodPost, "/tags/"+jobArn, map[string]any{
		"tags": map[string]string{"env": "test"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/tags/"+jobArn, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	tags, ok := resp["tags"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test", tags["env"])
}

// TestClassificationJobCreateFieldsRoundTrip locks the deferred
// ClassificationJob field audit: allowListIds/customDataIdentifierIds/
// managedDataIdentifierIds/managedDataIdentifierSelector must round-trip
// through Create -> Describe, and lastRunErrorStatus/statistics must be
// populated (DescribeClassificationJobOutput carries them once a job has
// started, which every job here does immediately on creation).
func TestClassificationJobCreateFieldsRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/jobs", map[string]any{
		"name":    "full-field-job",
		"jobType": "ONE_TIME",
		"s3JobDefinition": map[string]any{
			"bucketDefinitions": []any{},
		},
		"allowListIds":                  []string{"allow-1"},
		"customDataIdentifierIds":       []string{"cdi-1"},
		"managedDataIdentifierIds":      []string{"EMAIL_ADDRESS"},
		"managedDataIdentifierSelector": "INCLUDE",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var created map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	rec = doRequest(t, h, http.MethodGet, "/jobs/"+created["jobId"], nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var desc map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &desc))

	allowListIDs, ok := desc["allowListIds"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"allow-1"}, allowListIDs)

	cdiIDs, ok := desc["customDataIdentifierIds"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"cdi-1"}, cdiIDs)

	mdiIDs, ok := desc["managedDataIdentifierIds"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"EMAIL_ADDRESS"}, mdiIDs)

	assert.Equal(t, "INCLUDE", desc["managedDataIdentifierSelector"])

	lastRunErrorStatus, ok := desc["lastRunErrorStatus"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "NONE", lastRunErrorStatus["code"])

	stats, ok := desc["statistics"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, stats, "numberOfRuns")
}
