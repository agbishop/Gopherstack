package batch_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/batch"
)

func TestHandler_JobQueue_CRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *batch.Handler)
		name       string
		wantStatus int
		wantARN    bool
	}{
		{
			name:       "create_success",
			wantStatus: http.StatusOK,
			wantARN:    true,
		},
		{
			name: "create_duplicate",
			setup: func(t *testing.T, h *batch.Handler) {
				t.Helper()
				rec := post(t, h, "/v1/createjobqueue", map[string]any{
					"jobQueueName": "test-jq",
					"priority":     10,
					"state":        "ENABLED",
				})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := post(t, h, "/v1/createjobqueue", map[string]any{
				"jobQueueName": "test-jq",
				"priority":     10,
				"state":        "ENABLED",
			})

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantARN {
				var out map[string]string
				mustUnmarshal(t, rec, &out)
				assert.Contains(t, out["jobQueueArn"], "test-jq")
				assert.Equal(t, "test-jq", out["jobQueueName"])
			}
		})
	}
}

func TestHandler_DescribeJobQueues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		filter     []string
		wantCount  int
		wantStatus int
	}{
		{name: "describe_all", filter: nil, wantCount: 2, wantStatus: http.StatusOK},
		{name: "describe_one", filter: []string{"jq-1"}, wantCount: 1, wantStatus: http.StatusOK},
		{name: "describe_missing", filter: []string{"no-such-queue"}, wantCount: 0, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			for _, name := range []string{"jq-1", "jq-2"} {
				rec := post(t, h, "/v1/createjobqueue", map[string]any{
					"jobQueueName": name,
					"priority":     1,
				})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			body := map[string]any{}
			if tt.filter != nil {
				body["jobQueues"] = tt.filter
			}

			rec := post(t, h, "/v1/describejobqueues", body)

			require.Equal(t, tt.wantStatus, rec.Code)

			var out map[string]any
			mustUnmarshal(t, rec, &out)

			list, ok := out["jobQueues"].([]any)
			require.True(t, ok)
			assert.Len(t, list, tt.wantCount)
		})
	}
}

func TestHandler_UpdateJobQueue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		priority   *int32
		name       string
		jq         string
		state      string
		wantStatus int
	}{
		{
			name:       "update_state",
			jq:         "test-jq",
			state:      "DISABLED",
			wantStatus: http.StatusOK,
		},
		{
			name: "update_priority",
			jq:   "test-jq",
			priority: func() *int32 {
				v := int32(20)

				return &v
			}(),
			wantStatus: http.StatusOK,
		},
		{
			name:       "not_found",
			jq:         "no-such-queue",
			state:      "DISABLED",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := post(t, h, "/v1/createjobqueue", map[string]any{
				"jobQueueName": "test-jq",
				"priority":     10,
				"state":        "ENABLED",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			body := map[string]any{"jobQueue": tt.jq}
			if tt.state != "" {
				body["state"] = tt.state
			}

			if tt.priority != nil {
				body["priority"] = *tt.priority
			}

			rec = post(t, h, "/v1/updatejobqueue", body)

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_DeleteJobQueue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		jq         string
		wantStatus int
	}{
		{name: "delete_success", jq: "test-jq", wantStatus: http.StatusOK},
		{name: "delete_not_found", jq: "missing", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := post(t, h, "/v1/createjobqueue", map[string]any{
				"jobQueueName": "test-jq",
				"priority":     1,
			})
			require.Equal(t, http.StatusOK, rec.Code)

			if tt.jq == "test-jq" {
				rec = post(t, h, "/v1/updatejobqueue", map[string]any{
					"jobQueue": "test-jq",
					"state":    "DISABLED",
				})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec = post(t, h, "/v1/deletejobqueue", map[string]any{
				"jobQueue": tt.jq,
			})

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// --- Job Definition tests ---

func TestHandler_JobQueueWithComputeEnvironmentOrder(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := post(t, h, "/v1/createjobqueue", map[string]any{
		"jobQueueName": "ordered-jq",
		"priority":     5,
		"state":        "ENABLED",
		"computeEnvironmentOrder": []map[string]any{
			{"computeEnvironment": "ce-1", "order": 1},
			{"computeEnvironment": "ce-2", "order": 2},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/describejobqueues", map[string]any{
		"jobQueues": []string{"ordered-jq"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	mustUnmarshal(t, rec, &out)
	jqs := out["jobQueues"].([]any)
	require.Len(t, jqs, 1)

	jq := jqs[0].(map[string]any)
	ceOrder := jq["computeEnvironmentOrder"].([]any)
	assert.Len(t, ceOrder, 2)
}

// TestHandler_JobQueue_ServiceEnvironmentOrder verifies CreateJobQueue and
// UpdateJobQueue accept and surface jobQueueType and serviceEnvironmentOrder
// (SAGEMAKER_TRAINING service-job queues use these instead of
// computeEnvironmentOrder), and that mixing both order types in a single
// create is rejected -- matching aws-sdk-go-v2/service/batch's documented
// "a job queue can't have both a serviceEnvironmentOrder and a
// computeEnvironmentOrder field" constraint. Both fields were previously
// entirely unmodeled.
func TestHandler_JobQueue_ServiceEnvironmentOrder(t *testing.T) {
	t.Parallel()

	t.Run("create_and_describe", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)

		rec := post(t, h, "/v1/createjobqueue", map[string]any{
			"jobQueueName": "sagemaker-jq",
			"priority":     1,
			"state":        "ENABLED",
			"jobQueueType": "SAGEMAKER_TRAINING",
			"serviceEnvironmentOrder": []map[string]any{
				{"serviceEnvironment": "se-1", "order": 1},
			},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		rec = post(t, h, "/v1/describejobqueues", map[string]any{
			"jobQueues": []string{"sagemaker-jq"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var out map[string]any
		mustUnmarshal(t, rec, &out)
		jq := out["jobQueues"].([]any)[0].(map[string]any)
		assert.Equal(t, "SAGEMAKER_TRAINING", jq["jobQueueType"])

		seOrder := jq["serviceEnvironmentOrder"].([]any)
		require.Len(t, seOrder, 1)
		assert.Equal(t, "se-1", seOrder[0].(map[string]any)["serviceEnvironment"])
	})

	t.Run("mixed_order_types_rejected", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)

		rec := post(t, h, "/v1/createjobqueue", map[string]any{
			"jobQueueName": "mixed-jq",
			"priority":     1,
			"state":        "ENABLED",
			"computeEnvironmentOrder": []map[string]any{
				{"computeEnvironment": "ce-1", "order": 1},
			},
			"serviceEnvironmentOrder": []map[string]any{
				{"serviceEnvironment": "se-1", "order": 1},
			},
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestHandler_JobQueueByARN(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := post(t, h, "/v1/createjobqueue", map[string]any{
		"jobQueueName": "arn-lookup-jq",
		"priority":     1,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]string
	mustUnmarshal(t, rec, &out)
	jqARN := out["jobQueueArn"]

	// Update by ARN.
	rec = post(t, h, "/v1/updatejobqueue", map[string]any{
		"jobQueue": jqARN,
		"state":    "DISABLED",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Delete by ARN.
	rec = post(t, h, "/v1/deletejobqueue", map[string]any{
		"jobQueue": jqARN,
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_DeleteJobQueue_TerminatesJobs(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create compute environment and job queue.
	rec := post(t, h, "/v1/createcomputeenvironment", map[string]any{
		"computeEnvironmentName": "env1",
		"type":                   "MANAGED",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/createjobqueue", map[string]any{
		"jobQueueName": "q1",
		"priority":     1,
		"computeEnvironmentOrder": []map[string]any{
			{"order": 1, "computeEnvironment": "env1"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/registerjobdefinition", map[string]any{
		"jobDefinitionName": "jd1",
		"type":              "container",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Submit a job.
	rec = post(t, h, "/v1/submitjob", map[string]any{
		"jobName":       "job1",
		"jobQueue":      "q1",
		"jobDefinition": "jd1",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var submitResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &submitResp))
	jobID := submitResp["jobId"].(string)

	// Delete the queue — associated jobs must be terminated (FAILED), not
	// vanish. AWS: "All jobs in the queue are eventually terminated when you
	// delete a job queue" (api_op_DeleteJobQueue.go) -- job history stays
	// describable by ID, same as a real TerminateJob.
	rec = post(t, h, "/v1/updatejobqueue", map[string]any{
		"jobQueue": "q1",
		"state":    "DISABLED",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/deletejobqueue", map[string]any{
		"jobQueue": "q1",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/describejobs", map[string]any{
		"jobs": []string{jobID},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	jobs := descResp["jobs"].([]any)
	require.Len(t, jobs, 1, "job history must survive job queue deletion")
	job, _ := jobs[0].(map[string]any)
	assert.Equal(t, "FAILED", job["status"])
}

func TestHandler_GetJobQueueSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		queueName   string
		wantStatus  int
		createQueue bool
	}{
		{
			name:        "valid_queue",
			wantStatus:  http.StatusOK,
			createQueue: true,
		},
		{
			name:       "missing_queue",
			queueName:  "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "not_found",
			queueName:  "nonexistent-queue",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			queueName := tt.queueName

			if tt.createQueue {
				ceName := "ce-snap-" + tt.name
				rec := post(t, h, "/v1/createcomputeenvironment", map[string]any{
					"computeEnvironmentName": ceName,
					"type":                   "MANAGED",
					"state":                  "ENABLED",
				})
				require.Equal(t, http.StatusOK, rec.Code)

				queueName = "q-snap-" + tt.name
				rec = post(t, h, "/v1/createjobqueue", map[string]any{
					"jobQueueName": queueName,
					"priority":     1,
					"state":        "ENABLED",
					"computeEnvironmentOrder": []map[string]any{
						{"computeEnvironment": ceName, "order": 1},
					},
				})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := post(t, h, "/v1/getjobqueuesnapshot", map[string]any{"jobQueue": queueName})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var out map[string]any
				mustUnmarshal(t, rec, &out)
				assert.Contains(t, out, "frontOfQueue")
			}
		})
	}
}

// TestHandler_GetJobQueueSnapshot_WireShape verifies GetJobQueueSnapshot's
// response uses aws-sdk-go-v2/service/batch/types.FrontOfQueueDetail's exact
// field names: "lastUpdatedAt" (not "timestamp") and each job's
// "earliestTimeAtPosition" as an epoch-millisecond number (not a
// seconds-based float division) -- see PARITY.md gaps.
func TestHandler_GetJobQueueSnapshot_WireShape(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := post(t, h, "/v1/createcomputeenvironment", map[string]any{
		"computeEnvironmentName": "ce-snap-wire",
		"type":                   "MANAGED",
		"state":                  "ENABLED",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/createjobqueue", map[string]any{
		"jobQueueName": "q-snap-wire",
		"priority":     1,
		"state":        "ENABLED",
		"computeEnvironmentOrder": []map[string]any{
			{"computeEnvironment": "ce-snap-wire", "order": 1},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/registerjobdefinition", map[string]any{
		"jobDefinitionName": "jd-snap-wire", "type": "container",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/submitjob", map[string]any{
		"jobName": "job-snap-wire", "jobQueue": "q-snap-wire", "jobDefinition": "jd-snap-wire",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var submitOut map[string]any
	mustUnmarshal(t, rec, &submitOut)
	jobID := submitOut["jobId"].(string)

	// GetJobQueueSnapshot only surfaces RUNNABLE jobs; force the freshly
	// submitted (SUBMITTED-status) job into RUNNABLE for this test.
	h.Backend.ForceJobStatus(jobID, "RUNNABLE")

	rec = post(t, h, "/v1/getjobqueuesnapshot", map[string]any{"jobQueue": "q-snap-wire"})
	require.Equal(t, http.StatusOK, rec.Code)

	var rawMap map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rawMap))

	var frontOfQueue map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rawMap["frontOfQueue"], &frontOfQueue))

	_, hasTimestamp := frontOfQueue["timestamp"]
	assert.False(t, hasTimestamp, "wire shape must not have a legacy \"timestamp\" field")

	var lastUpdatedAt int64
	require.NoError(t, json.Unmarshal(frontOfQueue["lastUpdatedAt"], &lastUpdatedAt))
	assert.Positive(t, lastUpdatedAt, "lastUpdatedAt should be a positive epoch-millisecond value")

	var jobs []map[string]any
	require.NoError(t, json.Unmarshal(frontOfQueue["jobs"], &jobs))
	require.Len(t, jobs, 1)
	assert.Contains(t, jobs[0]["jobArn"], "job/")

	earliestTimeAtPosition, ok := jobs[0]["earliestTimeAtPosition"].(float64)
	require.True(t, ok)
	assert.Greater(
		t, earliestTimeAtPosition, float64(1e12),
		"earliestTimeAtPosition should be epoch-milliseconds, not epoch-seconds",
	)
}

// TestDescribeJobQueues_TagsPresentNoTags verifies that
// DescribeJobQueues always includes "tags": {} when a queue has no tags.
func TestDescribeJobQueues_TagsPresentNoTags(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := post(t, h, "/v1/createcomputeenvironment", map[string]any{
		"computeEnvironmentName": "ce-for-jq",
		"type":                   "MANAGED",
		"state":                  "ENABLED",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/createjobqueue", map[string]any{
		"jobQueueName": "jq-notags",
		"priority":     1,
		"state":        "ENABLED",
		"computeEnvironmentOrder": []map[string]any{
			{"computeEnvironment": "ce-for-jq", "order": 1},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/describejobqueues", map[string]any{
		"jobQueues": []string{"jq-notags"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	mustUnmarshal(t, rec, &out)
	items := out["jobQueues"].([]any)
	require.Len(t, items, 1)

	itemBytes, err := json.Marshal(items[0])
	require.NoError(t, err)
	assertTagsPresent(t, itemBytes)
}

// TestDescribeJobQueues_EmptyList verifies that DescribeJobQueues
// returns "jobQueues": [] not null.
func TestDescribeJobQueues_EmptyList(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := post(t, h, "/v1/describejobqueues", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var rawMap map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rawMap))
	raw, ok := rawMap["jobQueues"]
	require.True(t, ok, "jobQueues key must be present")
	assert.Equal(t, "[]", string(raw), "jobQueues must be [] not null when empty")
}

// quotaShareCreateInput builds a valid CreateQuotaShare request body
// referencing jobQueue, so each subtest below can vary a single field.
func quotaShareCreateInput(name, jobQueue string) map[string]any {
	return map[string]any{
		"quotaShareName": name,
		"jobQueue":       jobQueue,
		"capacityLimits": []map[string]any{
			{"capacityUnit": "ml.m5.large", "maxCapacity": 10},
		},
		"preemptionConfiguration": map[string]any{"inSharePreemption": "ENABLED"},
		"resourceSharingConfiguration": map[string]any{
			"strategy":    "LEND_AND_BORROW",
			"borrowLimit": 200,
		},
	}
}

// TestHandler_QuotaShare_Create covers CreateQuotaShare's real-API
// validation: it must reference an existing job queue (jobQueue is a
// required field on CreateQuotaShareInput, and real AWS Batch requires the
// queue to already exist), duplicate names on the same queue are rejected,
// and the enum fields (state/inSharePreemption/strategy) are validated
// against their real documented values.
func TestHandler_QuotaShare_Create(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mutate     func(body map[string]any)
		name       string
		wantStatus int
		duplicate  bool
	}{
		{name: "valid_create", wantStatus: http.StatusOK},
		{
			name:       "unknown_job_queue",
			wantStatus: http.StatusBadRequest,
			mutate:     func(body map[string]any) { body["jobQueue"] = "does-not-exist" },
		},
		{
			name:       "invalid_state",
			wantStatus: http.StatusBadRequest,
			mutate:     func(body map[string]any) { body["state"] = "BOGUS" },
		},
		{
			name:       "invalid_preemption",
			wantStatus: http.StatusBadRequest,
			mutate: func(body map[string]any) {
				body["preemptionConfiguration"] = map[string]any{"inSharePreemption": "BOGUS"}
			},
		},
		{
			name:       "invalid_strategy",
			wantStatus: http.StatusBadRequest,
			mutate: func(body map[string]any) {
				body["resourceSharingConfiguration"] = map[string]any{"strategy": "BOGUS"}
			},
		},
		{
			name:       "duplicate_name",
			wantStatus: http.StatusBadRequest,
			duplicate:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			qName := createTestServiceJobQueue(t, h, "qs-create-"+tt.name)

			body := quotaShareCreateInput("qs-1", qName)
			if tt.mutate != nil {
				tt.mutate(body)
			}

			if tt.duplicate {
				rec := post(t, h, "/v1/createquotashare", body)
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := post(t, h, "/v1/createquotashare", body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var out map[string]any
				mustUnmarshal(t, rec, &out)
				assert.Equal(t, "qs-1", out["quotaShareName"])
				assert.Contains(t, out["quotaShareArn"], "quota-share/qs-1")
				assert.Contains(t, out["quotaShareArn"], qName)
			}
		})
	}
}

// TestHandler_QuotaShare_Lifecycle exercises DescribeQuotaShare,
// UpdateQuotaShare, DeleteQuotaShare, and ListQuotaShares against a quota
// share created fresh per subtest, proving each op reads/mutates the same
// real record (not a fresh/parallel one) and that ListQuotaShares' item
// shape (quotaShareDetail) omits tags while DescribeQuotaShare's includes
// them.
func TestHandler_QuotaShare_Lifecycle(t *testing.T) {
	t.Parallel()

	t.Run("describe_round_trip", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		qName := createTestServiceJobQueue(t, h, "qs-describe")

		body := quotaShareCreateInput("qs-describe", qName)
		body["tags"] = map[string]string{"team": "ml"}
		rec := post(t, h, "/v1/createquotashare", body)
		require.Equal(t, http.StatusOK, rec.Code)

		var created map[string]any
		mustUnmarshal(t, rec, &created)
		arnStr := created["quotaShareArn"].(string)

		recD := post(t, h, "/v1/describequotashare", map[string]any{"quotaShareArn": arnStr})
		require.Equal(t, http.StatusOK, recD.Code)

		var desc map[string]any
		mustUnmarshal(t, recD, &desc)
		assert.Equal(t, "qs-describe", desc["quotaShareName"])
		assert.Equal(t, "ENABLED", desc["state"])
		assert.Equal(t, "VALID", desc["status"])
		assert.Contains(t, desc["jobQueueArn"], qName)
		assertTagsPresent(t, recD.Body.Bytes())
		tags := desc["tags"].(map[string]any)
		assert.Equal(t, "ml", tags["team"])
	})

	t.Run("describe_not_found", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := post(t, h, "/v1/describequotashare", map[string]any{
			"quotaShareArn": "arn:aws:batch:us-east-1:000000000000:job-queue/x/quota-share/y",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("update_mutates_existing_record", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		qName := createTestServiceJobQueue(t, h, "qs-update")

		rec := post(t, h, "/v1/createquotashare", quotaShareCreateInput("qs-update", qName))
		require.Equal(t, http.StatusOK, rec.Code)

		var created map[string]any
		mustUnmarshal(t, rec, &created)
		arnStr := created["quotaShareArn"].(string)

		recU := post(t, h, "/v1/updatequotashare", map[string]any{
			"quotaShareArn": arnStr,
			"state":         "DISABLED",
			"capacityLimits": []map[string]any{
				{"capacityUnit": "ml.m5.xlarge", "maxCapacity": 20},
			},
		})
		require.Equal(t, http.StatusOK, recU.Code)

		var updated map[string]any
		mustUnmarshal(t, recU, &updated)
		assert.Equal(t, "qs-update", updated["quotaShareName"])
		assert.Equal(t, arnStr, updated["quotaShareArn"])

		// The same record now reflects the update, proving UpdateQuotaShare
		// mutated it in place rather than writing to a fresh/parallel store.
		recD := post(t, h, "/v1/describequotashare", map[string]any{"quotaShareArn": arnStr})
		require.Equal(t, http.StatusOK, recD.Code)

		var desc map[string]any
		mustUnmarshal(t, recD, &desc)
		assert.Equal(t, "DISABLED", desc["state"])
		limits := desc["capacityLimits"].([]any)
		require.Len(t, limits, 1)
		assert.Equal(t, "ml.m5.xlarge", limits[0].(map[string]any)["capacityUnit"])
	})

	t.Run("update_not_found", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := post(t, h, "/v1/updatequotashare", map[string]any{
			"quotaShareArn": "arn:aws:batch:us-east-1:000000000000:job-queue/x/quota-share/y",
			"state":         "DISABLED",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("delete_then_describe_not_found", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		qName := createTestServiceJobQueue(t, h, "qs-delete")

		rec := post(t, h, "/v1/createquotashare", quotaShareCreateInput("qs-delete", qName))
		require.Equal(t, http.StatusOK, rec.Code)

		var created map[string]any
		mustUnmarshal(t, rec, &created)
		arnStr := created["quotaShareArn"].(string)

		// AWS requires DISABLED before delete (api_op_DeleteQuotaShare.go).
		recU := post(t, h, "/v1/updatequotashare", map[string]any{
			"quotaShareArn": arnStr,
			"state":         "DISABLED",
		})
		require.Equal(t, http.StatusOK, recU.Code)

		recDel := post(t, h, "/v1/deletequotashare", map[string]any{"quotaShareArn": arnStr})
		assert.Equal(t, http.StatusOK, recDel.Code)

		recD := post(t, h, "/v1/describequotashare", map[string]any{"quotaShareArn": arnStr})
		assert.Equal(t, http.StatusBadRequest, recD.Code)
	})

	t.Run("delete_enabled_rejected", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		qName := createTestServiceJobQueue(t, h, "qs-delete-enabled")

		rec := post(t, h, "/v1/createquotashare", quotaShareCreateInput("qs-delete-enabled", qName))
		require.Equal(t, http.StatusOK, rec.Code)

		var created map[string]any
		mustUnmarshal(t, rec, &created)
		arnStr := created["quotaShareArn"].(string)

		recDel := post(t, h, "/v1/deletequotashare", map[string]any{"quotaShareArn": arnStr})
		assert.Equal(t, http.StatusBadRequest, recDel.Code)
	})

	t.Run("delete_not_found", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := post(t, h, "/v1/deletequotashare", map[string]any{
			"quotaShareArn": "arn:aws:batch:us-east-1:000000000000:job-queue/x/quota-share/y",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("list_scoped_to_job_queue", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		qA := createTestServiceJobQueue(t, h, "qs-list-a")
		qB := createTestServiceJobQueue(t, h, "qs-list-b")

		for _, name := range []string{"qs-list-1", "qs-list-2"} {
			rec := post(t, h, "/v1/createquotashare", quotaShareCreateInput(name, qA))
			require.Equal(t, http.StatusOK, rec.Code)
		}

		recList := post(t, h, "/v1/listquotashares", map[string]any{"jobQueue": qA})
		require.Equal(t, http.StatusOK, recList.Code)

		var out map[string]any
		mustUnmarshal(t, recList, &out)
		items := out["quotaShares"].([]any)
		require.Len(t, items, 2)

		// ListQuotaShares' item shape (quotaShareDetail) has no tags field at
		// all, unlike DescribeQuotaShare's output.
		first := items[0].(map[string]any)
		_, hasTags := first["tags"]
		assert.False(t, hasTags, "quotaShareDetail must not carry a tags field")

		// A different job queue with no quota shares returns an empty list.
		recEmpty := post(t, h, "/v1/listquotashares", map[string]any{"jobQueue": qB})
		require.Equal(t, http.StatusOK, recEmpty.Code)

		var outEmpty map[string]any
		mustUnmarshal(t, recEmpty, &outEmpty)
		assert.Empty(t, outEmpty["quotaShares"])
	})

	t.Run("list_unknown_job_queue", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := post(t, h, "/v1/listquotashares", map[string]any{"jobQueue": "does-not-exist"})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// TestHandler_UpdateJobQueue_SchedulingPolicyArn covers gopherstack-4shm's
// class directly: UpdateJobQueueInput.SchedulingPolicyArn is a real field
// (batch@v1.68.4 api_op_UpdateJobQueue.go: "the fair-share scheduling
// policy can be replaced but not removed") that the WrapOp-dispatched
// handler decoded but never passed to the backend at all. Asserts on the
// decoded DescribeJobQueues response, not just err == nil.
func TestHandler_UpdateJobQueue_SchedulingPolicyArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := post(t, h, "/v1/createjobqueue", map[string]any{
		"jobQueueName": "sched-jq",
		"priority":     10,
		"state":        "ENABLED",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	const wantArn = "aws:aws:batch:us-east-1:123456789012:scheduling-policy/MySchedulingPolicy"

	rec = post(t, h, "/v1/updatejobqueue", map[string]any{
		"jobQueue":            "sched-jq",
		"schedulingPolicyArn": wantArn,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = post(t, h, "/v1/describejobqueues", map[string]any{"jobQueues": []string{"sched-jq"}})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		JobQueues []struct {
			SchedulingPolicyArn string `json:"schedulingPolicyArn"`
		} `json:"jobQueues"`
	}
	mustUnmarshal(t, rec, &out)
	require.Len(t, out.JobQueues, 1)
	assert.Equal(t, wantArn, out.JobQueues[0].SchedulingPolicyArn)
}
