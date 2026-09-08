package batch_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/batch"
)

func TestHandler_JobOperations(t *testing.T) {
	t.Parallel()

	// Helper: set up a handler with a queue and job definition pre-created.
	// jdName is registered as a container job definition (revision 1) so that
	// SubmitJob's jobDefinition resolution (see backend.go
	// lookupJobDefinitionForSubmit) succeeds.
	newHandlerWithQueue := func(t *testing.T, queueName, jdName string) *batch.Handler {
		t.Helper()

		h := newTestHandler(t)
		ceBody := map[string]any{
			"computeEnvironmentName": "ce1",
			"type":                   "MANAGED",
			"state":                  "ENABLED",
		}
		rec := post(t, h, "/v1/createcomputeenvironment", ceBody)
		require.Equal(t, http.StatusOK, rec.Code, "create compute environment")

		jqBody := map[string]any{
			"jobQueueName": queueName,
			"priority":     1,
			"state":        "ENABLED",
			"computeEnvironmentOrder": []map[string]any{
				{"computeEnvironment": "ce1", "order": 1},
			},
		}
		rec = post(t, h, "/v1/createjobqueue", jqBody)
		require.Equal(t, http.StatusOK, rec.Code, "create job queue")

		rec = post(t, h, "/v1/registerjobdefinition", map[string]any{
			"jobDefinitionName": jdName,
			"type":              "container",
		})
		require.Equal(t, http.StatusOK, rec.Code, "register job definition")

		return h
	}

	t.Run("submit_list_describe_terminate", func(t *testing.T) {
		t.Parallel()

		h := newHandlerWithQueue(t, "my-queue", "my-jd")

		// SubmitJob succeeds when queue exists.
		submitRec := post(t, h, "/v1/submitjob", map[string]any{
			"jobName":       "my-job",
			"jobQueue":      "my-queue",
			"jobDefinition": "my-jd:1",
		})
		require.Equal(t, http.StatusOK, submitRec.Code)

		var submitOut map[string]any
		mustUnmarshal(t, submitRec, &submitOut)
		jobID, _ := submitOut["jobId"].(string)
		require.NotEmpty(t, jobID)

		// ListJobs returns the submitted job. The job is still SUBMITTED
		// (never scheduled); real AWS Batch's unfiltered ListJobs defaults to
		// RUNNING-only, so filter explicitly.
		listRec := post(t, h, "/v1/listjobs", map[string]any{"jobQueue": "my-queue", "jobStatus": "SUBMITTED"})
		require.Equal(t, http.StatusOK, listRec.Code)

		var listOut map[string]any
		mustUnmarshal(t, listRec, &listOut)
		summaries, _ := listOut["jobSummaryList"].([]any)
		assert.Len(t, summaries, 1)

		// DescribeJobs returns the full job detail.
		describeRec := post(t, h, "/v1/describejobs", map[string]any{"jobs": []string{jobID}})
		require.Equal(t, http.StatusOK, describeRec.Code)

		var describeOut map[string]any
		mustUnmarshal(t, describeRec, &describeOut)
		jobs, _ := describeOut["jobs"].([]any)
		assert.Len(t, jobs, 1)

		// TerminateJob marks the job FAILED.
		termRec := post(t, h, "/v1/terminatejob", map[string]any{"jobId": jobID, "reason": "test"})
		require.Equal(t, http.StatusOK, termRec.Code)
	})

	t.Run("cancel_job", func(t *testing.T) {
		t.Parallel()

		h := newHandlerWithQueue(t, "q2", "jd")

		submitRec := post(t, h, "/v1/submitjob", map[string]any{
			"jobName":       "j",
			"jobQueue":      "q2",
			"jobDefinition": "jd:1",
		})
		require.Equal(t, http.StatusOK, submitRec.Code)

		var submitOut map[string]any
		mustUnmarshal(t, submitRec, &submitOut)
		jobID, _ := submitOut["jobId"].(string)

		cancelRec := post(t, h, "/v1/canceljob", map[string]any{"jobId": jobID, "reason": "cancelled"})
		require.Equal(t, http.StatusOK, cancelRec.Code)
	})

	t.Run("describe_jobs_returns_empty_for_unknown", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := post(t, h, "/v1/describejobs", map[string]any{"jobs": []string{"unknown-id"}})
		require.Equal(t, http.StatusOK, rec.Code)

		var out map[string]any
		mustUnmarshal(t, rec, &out)
		jobs, _ := out["jobs"].([]any)
		assert.Empty(t, jobs)
	})

	t.Run("list_jobs_missing_queue_returns_error", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := post(t, h, "/v1/listjobs", map[string]any{"jobQueue": "nonexistent"})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("submit_job_missing_queue_returns_error", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := post(t, h, "/v1/submitjob", map[string]any{
			"jobName":       "j",
			"jobQueue":      "nonexistent",
			"jobDefinition": "jd:1",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("terminate_job_missing_returns_error", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := post(t, h, "/v1/terminatejob", map[string]any{"jobId": "nonexistent", "reason": "x"})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("cancel_job_missing_returns_error", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := post(t, h, "/v1/canceljob", map[string]any{"jobId": "nonexistent", "reason": "x"})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestHandler_SubmitJob_JobARNPresent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := post(t, h, "/v1/createcomputeenvironment", map[string]any{
		"computeEnvironmentName": "env-arn",
		"type":                   "MANAGED",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/createjobqueue", map[string]any{
		"jobQueueName": "q-arn",
		"priority":     1,
		"computeEnvironmentOrder": []map[string]any{
			{"order": 1, "computeEnvironment": "env-arn"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/registerjobdefinition", map[string]any{
		"jobDefinitionName": "jd1",
		"type":              "container",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/submitjob", map[string]any{
		"jobName":       "my-job",
		"jobQueue":      "q-arn",
		"jobDefinition": "jd1",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var submitResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &submitResp))
	jobID := submitResp["jobId"].(string)
	assert.NotEmpty(t, submitResp["jobArn"], "SubmitJob response should include jobArn")

	// DescribeJobs should return the job ARN.
	rec = post(t, h, "/v1/describejobs", map[string]any{
		"jobs": []string{jobID},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	jobs := descResp["jobs"].([]any)
	require.Len(t, jobs, 1)

	job := jobs[0].(map[string]any)
	assert.NotEmpty(t, job["jobArn"], "job ARN should be set")
	// JobDetail.JobQueue is documented as the queue's ARN, not its name.
	assert.Contains(t, job["jobQueue"], "job-queue/q-arn", "jobQueue should be the queue's ARN")
}

func TestHandler_SubmitJob_WithOptions(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	ceName := "ce-opt"
	rec := post(t, h, "/v1/createcomputeenvironment", map[string]any{
		"computeEnvironmentName": ceName, "type": "MANAGED", "state": "ENABLED",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	qName := "q-opt"
	rec = post(t, h, "/v1/createjobqueue", map[string]any{
		"jobQueueName": qName, "priority": 1, "state": "ENABLED",
		"computeEnvironmentOrder": []map[string]any{{"computeEnvironment": ceName, "order": 1}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	jdName := "jd-opt"
	rec = post(t, h, "/v1/registerjobdefinition", map[string]any{
		"jobDefinitionName": jdName, "type": "container",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	tests := []struct {
		input      map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "with_depends_on",
			input: map[string]any{
				"jobName": "job-dep", "jobQueue": qName, "jobDefinition": jdName,
				"dependsOn": []map[string]any{{"jobId": "fake-id", "type": "SEQUENTIAL"}},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "with_container_overrides",
			input: map[string]any{
				"jobName": "job-co", "jobQueue": qName, "jobDefinition": jdName,
				"containerOverrides": map[string]any{
					"command": []string{"echo", "hello"},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "with_parameters",
			input: map[string]any{
				"jobName": "job-params", "jobQueue": qName, "jobDefinition": jdName,
				"parameters": map[string]string{"key": "value"},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := post(t, h, "/v1/submitjob", tt.input)
			assert.Equal(t, tt.wantStatus, r.Code)
		})
	}
}

func TestHandler_CancelJob_NonCancellable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		preStatus     string
		wantJobStatus string
		wantStatus    int
	}{
		{
			name:          "cancel_submitted_succeeds",
			preStatus:     "SUBMITTED",
			wantJobStatus: "FAILED",
			wantStatus:    http.StatusOK,
		},
		{
			// AWS: "Jobs that progressed to the STARTING or RUNNING state
			// aren't canceled. However, the API operation still succeeds,
			// even if no job is canceled." (api_op_CancelJob.go)
			name:          "cancel_running_is_a_noop_that_still_succeeds",
			preStatus:     "RUNNING",
			wantJobStatus: "RUNNING",
			wantStatus:    http.StatusOK,
		},
		{
			name:          "cancel_starting_is_a_noop_that_still_succeeds",
			preStatus:     "STARTING",
			wantJobStatus: "STARTING",
			wantStatus:    http.StatusOK,
		},
		{
			name:          "cancel_succeeded_fails",
			preStatus:     "SUCCEEDED",
			wantJobStatus: "SUCCEEDED",
			wantStatus:    http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			h.Backend.Reset()

			rec := post(t, h, "/v1/createcomputeenvironment", map[string]any{
				"computeEnvironmentName": "ce1",
				"type":                   "MANAGED",
				"state":                  "ENABLED",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			rec = post(t, h, "/v1/createjobqueue", map[string]any{
				"jobQueueName": "q1",
				"priority":     1,
			})
			require.Equal(t, http.StatusOK, rec.Code)

			rec = post(t, h, "/v1/registerjobdefinition", map[string]any{
				"jobDefinitionName": "jd",
				"type":              "container",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			rec = post(t, h, "/v1/submitjob", map[string]any{
				"jobName":       "test-job",
				"jobQueue":      "q1",
				"jobDefinition": "jd:1",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var submitOut map[string]any
			mustUnmarshal(t, rec, &submitOut)
			jobID := submitOut["jobId"].(string)

			if tt.preStatus != "SUBMITTED" {
				h.Backend.ForceJobStatus(jobID, tt.preStatus)
			}

			rec = post(t, h, "/v1/canceljob", map[string]any{
				"jobId":  jobID,
				"reason": "test",
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			describeRec := post(t, h, "/v1/describejobs", map[string]any{"jobs": []string{jobID}})
			require.Equal(t, http.StatusOK, describeRec.Code)

			var describeOut map[string]any
			mustUnmarshal(t, describeRec, &describeOut)
			jobs, _ := describeOut["jobs"].([]any)
			require.Len(t, jobs, 1)
			job, _ := jobs[0].(map[string]any)
			assert.Equal(t, tt.wantJobStatus, job["status"])
		})
	}
}

func TestHandler_TerminateJob_Terminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		preStatus  string
		wantStatus int
	}{
		{
			name:       "terminate_running_succeeds",
			preStatus:  "RUNNING",
			wantStatus: http.StatusOK,
		},
		{
			name:       "terminate_succeeded_fails",
			preStatus:  "SUCCEEDED",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "terminate_failed_fails",
			preStatus:  "FAILED",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			h.Backend.Reset()

			rec := post(t, h, "/v1/createcomputeenvironment", map[string]any{
				"computeEnvironmentName": "ce1",
				"type":                   "MANAGED",
				"state":                  "ENABLED",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			rec = post(t, h, "/v1/createjobqueue", map[string]any{
				"jobQueueName": "q1",
				"priority":     1,
			})
			require.Equal(t, http.StatusOK, rec.Code)

			rec = post(t, h, "/v1/registerjobdefinition", map[string]any{
				"jobDefinitionName": "jd",
				"type":              "container",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			rec = post(t, h, "/v1/submitjob", map[string]any{
				"jobName":       "test-job",
				"jobQueue":      "q1",
				"jobDefinition": "jd:1",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var submitOut map[string]any
			mustUnmarshal(t, rec, &submitOut)
			jobID := submitOut["jobId"].(string)

			h.Backend.ForceJobStatus(jobID, tt.preStatus)

			rec = post(t, h, "/v1/terminatejob", map[string]any{
				"jobId":  jobID,
				"reason": "test",
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_SubmitJob_InvalidName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		jobName    string
		wantStatus int
	}{
		{name: "empty_name_fails", jobName: "", wantStatus: http.StatusBadRequest},
		{name: "too_long_name_fails", jobName: strings.Repeat("a", 129), wantStatus: http.StatusBadRequest},
		{name: "valid_name_succeeds", jobName: "valid-job", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			h.Backend.Reset()

			rec := post(t, h, "/v1/createcomputeenvironment", map[string]any{
				"computeEnvironmentName": "ce1",
				"type":                   "MANAGED",
				"state":                  "ENABLED",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			rec = post(t, h, "/v1/createjobqueue", map[string]any{
				"jobQueueName": "q1",
				"priority":     1,
			})
			require.Equal(t, http.StatusOK, rec.Code)

			rec = post(t, h, "/v1/registerjobdefinition", map[string]any{
				"jobDefinitionName": "jd",
				"type":              "container",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			rec = post(t, h, "/v1/submitjob", map[string]any{
				"jobName":       tt.jobName,
				"jobQueue":      "q1",
				"jobDefinition": "jd:1",
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_ListJobs_NoQueue(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := post(t, h, "/v1/createcomputeenvironment", map[string]any{
		"computeEnvironmentName": "ce1",
		"type":                   "MANAGED",
		"state":                  "ENABLED",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/createjobqueue", map[string]any{
		"jobQueueName": "q1",
		"priority":     1,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/registerjobdefinition", map[string]any{
		"jobDefinitionName": "jd",
		"type":              "container",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/submitjob", map[string]any{
		"jobName":       "job1",
		"jobQueue":      "q1",
		"jobDefinition": "jd:1",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// AWS Batch ListJobs requires a grouping key (jobQueue here); without one it
	// returns a ClientException (HTTP 400), it does not list all jobs.
	rec = post(t, h, "/v1/listjobs", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// With no jobStatus filter, real AWS Batch returns only RUNNING jobs
	// (api_op_ListJobs.go: "If you don't specify a status, only RUNNING jobs
	// are returned"). The submitted job is still SUBMITTED, so it's excluded.
	rec = post(t, h, "/v1/listjobs", map[string]any{"jobQueue": "q1"})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	mustUnmarshal(t, rec, &out)
	summaries, _ := out["jobSummaryList"].([]any)
	assert.Empty(t, summaries)

	// Filtering explicitly by the job's actual status still finds it.
	rec = post(t, h, "/v1/listjobs", map[string]any{"jobQueue": "q1", "jobStatus": "SUBMITTED"})
	require.Equal(t, http.StatusOK, rec.Code)

	mustUnmarshal(t, rec, &out)
	summaries, _ = out["jobSummaryList"].([]any)
	assert.NotEmpty(t, summaries)
}

// newAuditHandlerWithQueue returns a handler pre-loaded with a CE, job queue,
// and job definition named "audit-jd" for use in job-submission audit tests.
func newAuditHandlerWithQueue(t *testing.T, queueName string) *batch.Handler {
	t.Helper()

	h := newTestHandler(t)

	rec := post(t, h, "/v1/createcomputeenvironment", map[string]any{
		"computeEnvironmentName": "audit-ce",
		"type":                   "MANAGED",
		"state":                  "ENABLED",
	})
	require.Equal(t, http.StatusOK, rec.Code, "create CE")

	rec = post(t, h, "/v1/createjobqueue", map[string]any{
		"jobQueueName": queueName,
		"priority":     1,
		"state":        "ENABLED",
		"computeEnvironmentOrder": []map[string]any{
			{"computeEnvironment": "audit-ce", "order": 1},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, "create job queue")

	rec = post(t, h, "/v1/registerjobdefinition", map[string]any{
		"jobDefinitionName": "audit-jd",
		"type":              "container",
	})
	require.Equal(t, http.StatusOK, rec.Code, "register job definition")

	return h
}

// TestDescribeJobs_TagsPresentNoTags verifies that DescribeJobs
// always includes "tags": {} when a job was submitted without tags.
func TestDescribeJobs_TagsPresentNoTags(t *testing.T) {
	t.Parallel()

	h := newAuditHandlerWithQueue(t, "audit-q-notags")

	rec := post(t, h, "/v1/submitjob", map[string]any{
		"jobName":       "job-notags",
		"jobQueue":      "audit-q-notags",
		"jobDefinition": "audit-jd",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var submitOut map[string]string
	mustUnmarshal(t, rec, &submitOut)

	rec = post(t, h, "/v1/describejobs", map[string]any{
		"jobs": []string{submitOut["jobId"]},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	mustUnmarshal(t, rec, &out)
	jobs := out["jobs"].([]any)
	require.Len(t, jobs, 1)

	itemBytes, err := json.Marshal(jobs[0])
	require.NoError(t, err)
	assertTagsPresent(t, itemBytes)
}

// TestDescribeJobs_TagsRoundTrip verifies that tags submitted
// with a job are returned in DescribeJobs.
func TestDescribeJobs_TagsRoundTrip(t *testing.T) {
	t.Parallel()

	h := newAuditHandlerWithQueue(t, "audit-q-tags")

	rec := post(t, h, "/v1/submitjob", map[string]any{
		"jobName":       "job-withtags",
		"jobQueue":      "audit-q-tags",
		"jobDefinition": "audit-jd",
		"tags":          map[string]string{"team": "infra", "env": "test"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var submitOut map[string]string
	mustUnmarshal(t, rec, &submitOut)

	rec = post(t, h, "/v1/describejobs", map[string]any{
		"jobs": []string{submitOut["jobId"]},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	mustUnmarshal(t, rec, &out)
	jobs := out["jobs"].([]any)
	require.Len(t, jobs, 1)
	tags := jobs[0].(map[string]any)["tags"].(map[string]any)
	assert.Equal(t, "infra", tags["team"])
	assert.Equal(t, "test", tags["env"])
}

// TestDescribeJobs_EmptyList verifies that DescribeJobs returns
// "jobs": [] not null when no matching jobs are found.
func TestDescribeJobs_EmptyList(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := post(t, h, "/v1/describejobs", map[string]any{
		"jobs": []string{"nonexistent-job-id"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var rawMap map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rawMap))
	raw, ok := rawMap["jobs"]
	require.True(t, ok, "jobs key must be present")
	assert.Equal(t, "[]", string(raw), "jobs must be [] not null when no matching jobs found")
}

// TestListJobs_SummaryIncludesJobArn verifies that ListJobs returns
// jobArn in each jobSummary. AWS always populates jobArn on job summaries.
func TestListJobs_SummaryIncludesJobArn(t *testing.T) {
	t.Parallel()

	h := newAuditHandlerWithQueue(t, "audit-q-arn")

	rec := post(t, h, "/v1/submitjob", map[string]any{
		"jobName":       "job-for-arn-check",
		"jobQueue":      "audit-q-arn",
		"jobDefinition": "audit-jd",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// The job is still SUBMITTED (never scheduled); real AWS Batch's
	// unfiltered ListJobs defaults to RUNNING-only, so filter explicitly.
	rec = post(t, h, "/v1/listjobs", map[string]any{"jobQueue": "audit-q-arn", "jobStatus": "SUBMITTED"})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	summaries, _ := out["jobSummaryList"].([]any)
	require.Len(t, summaries, 1)

	s := summaries[0].(map[string]any)
	arn, hasARN := s["jobArn"].(string)
	assert.True(t, hasARN && arn != "", "jobArn must be present and non-empty in ListJobs summary")
	assert.Contains(t, arn, "arn:aws:batch:", "jobArn must be a valid ARN")
}

// TestListJobs_SummaryIncludesTimestamps verifies that ListJobs
// returns startedAt and stoppedAt in jobSummary entries when the job has
// transitioned through those states. AWS populates these fields.
func TestListJobs_SummaryIncludesTimestamps(t *testing.T) {
	t.Parallel()

	h := newAuditHandlerWithQueue(t, "audit-q-ts")

	rec := post(t, h, "/v1/submitjob", map[string]any{
		"jobName":       "job-for-ts-check",
		"jobQueue":      "audit-q-ts",
		"jobDefinition": "audit-jd",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var submitOut map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &submitOut))
	jobID := submitOut["jobId"]

	rec = post(t, h, "/v1/terminatejob", map[string]any{
		"jobId":  jobID,
		"reason": "test termination",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/listjobs", map[string]any{
		"jobQueue":  "audit-q-ts",
		"jobStatus": "FAILED",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	summaries, _ := out["jobSummaryList"].([]any)
	require.Len(t, summaries, 1)

	s := summaries[0].(map[string]any)
	stoppedAt, hasStoppedAt := s["stoppedAt"].(float64)
	assert.True(t, hasStoppedAt && stoppedAt > 0, "stoppedAt must be present and positive after termination")
}

// TestListJobs_InvalidJobStatus verifies that ListJobs returns
// HTTP 400 when an unrecognised jobStatus is provided.
// AWS Batch returns ClientException for invalid status values.
func TestListJobs_InvalidJobStatus(t *testing.T) {
	t.Parallel()

	h := newAuditHandlerWithQueue(t, "audit-q-badstatus")

	rec := post(t, h, "/v1/listjobs", map[string]any{
		"jobQueue":  "audit-q-badstatus",
		"jobStatus": "INVALID_STATUS",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "invalid jobStatus must return 400")
}

// TestDescribeJobs_ExecutionDetails verifies the previously-unmodeled
// JobDetail fields (see PARITY.md gaps): container (derived from the job
// definition's ContainerProperties), platformCapabilities (snapshotted from
// the job definition at submit time), and isCancelled/isTerminated (set by
// CancelJob/TerminateJob respectively).
func TestDescribeJobs_ExecutionDetails(t *testing.T) {
	t.Parallel()

	t.Run("container_and_platform_capabilities", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)

		rec := post(t, h, "/v1/createcomputeenvironment", map[string]any{
			"computeEnvironmentName": "ce-exec", "type": "MANAGED", "state": "ENABLED",
		})
		require.Equal(t, http.StatusOK, rec.Code)

		rec = post(t, h, "/v1/createjobqueue", map[string]any{
			"jobQueueName": "q-exec", "priority": 1, "state": "ENABLED",
			"computeEnvironmentOrder": []map[string]any{{"computeEnvironment": "ce-exec", "order": 1}},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		rec = post(t, h, "/v1/registerjobdefinition", map[string]any{
			"jobDefinitionName":    "jd-exec",
			"type":                 "container",
			"platformCapabilities": []string{"EC2"},
			"containerProperties": map[string]any{
				"image": "busybox", "vcpus": 1, "memory": 128,
			},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		rec = post(t, h, "/v1/submitjob", map[string]any{
			"jobName": "job-exec", "jobQueue": "q-exec", "jobDefinition": "jd-exec",
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var submitOut map[string]any
		mustUnmarshal(t, rec, &submitOut)
		jobID := submitOut["jobId"].(string)

		rec = post(t, h, "/v1/describejobs", map[string]any{"jobs": []string{jobID}})
		require.Equal(t, http.StatusOK, rec.Code)

		var out map[string]any
		mustUnmarshal(t, rec, &out)
		jobs := out["jobs"].([]any)
		require.Len(t, jobs, 1)

		job := jobs[0].(map[string]any)
		caps, ok := job["platformCapabilities"].([]any)
		require.True(t, ok, "platformCapabilities should be present")
		assert.Equal(t, []any{"EC2"}, caps)

		container, ok := job["container"].(map[string]any)
		require.True(t, ok, "container should be derived from the job definition")
		assert.Equal(t, "busybox", container["image"])
		assert.InEpsilon(t, float64(1), container["vcpus"].(float64), 0.001)
	})

	t.Run("is_cancelled_set_by_cancel_job", func(t *testing.T) {
		t.Parallel()

		h := newAuditHandlerWithQueue(t, "q-cancel-flag")

		rec := post(t, h, "/v1/submitjob", map[string]any{
			"jobName": "job-cancel-flag", "jobQueue": "q-cancel-flag", "jobDefinition": "audit-jd",
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var submitOut map[string]any
		mustUnmarshal(t, rec, &submitOut)
		jobID := submitOut["jobId"].(string)

		rec = post(t, h, "/v1/canceljob", map[string]any{"jobId": jobID, "reason": "test"})
		require.Equal(t, http.StatusOK, rec.Code)

		rec = post(t, h, "/v1/describejobs", map[string]any{"jobs": []string{jobID}})
		require.Equal(t, http.StatusOK, rec.Code)

		var out map[string]any
		mustUnmarshal(t, rec, &out)
		job := out["jobs"].([]any)[0].(map[string]any)
		assert.Equal(t, true, job["isCancelled"])
		assert.Equal(t, false, job["isTerminated"])
	})

	t.Run("is_terminated_set_by_terminate_job", func(t *testing.T) {
		t.Parallel()

		h := newAuditHandlerWithQueue(t, "q-terminate-flag")

		rec := post(t, h, "/v1/submitjob", map[string]any{
			"jobName": "job-terminate-flag", "jobQueue": "q-terminate-flag", "jobDefinition": "audit-jd",
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var submitOut map[string]any
		mustUnmarshal(t, rec, &submitOut)
		jobID := submitOut["jobId"].(string)

		rec = post(t, h, "/v1/terminatejob", map[string]any{"jobId": jobID, "reason": "test"})
		require.Equal(t, http.StatusOK, rec.Code)

		rec = post(t, h, "/v1/describejobs", map[string]any{"jobs": []string{jobID}})
		require.Equal(t, http.StatusOK, rec.Code)

		var out map[string]any
		mustUnmarshal(t, rec, &out)
		job := out["jobs"].([]any)[0].(map[string]any)
		assert.Equal(t, true, job["isTerminated"])
		assert.Equal(t, false, job["isCancelled"])
	})
}
