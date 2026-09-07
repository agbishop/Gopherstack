package dms_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/dms"
)

func TestAssessmentRun_Lifecycle(t *testing.T) {
	t.Parallel()

	// Helper to build RI + endpoints + task.
	setupTask := func(t *testing.T, h *dms.Handler, prefix string) string {
		t.Helper()

		riRec := doDMS(t, h, "CreateReplicationInstance", map[string]any{
			"ReplicationInstanceIdentifier": prefix + "-ri",
			"ReplicationInstanceClass":      "dms.t3.medium",
		})
		require.Equal(t, http.StatusOK, riRec.Code)
		riArn := parseJSON(t, riRec)["ReplicationInstance"].(map[string]any)["ReplicationInstanceArn"].(string)

		srcRec := doDMS(t, h, "CreateEndpoint", map[string]any{
			"EndpointIdentifier": prefix + "-src",
			"EndpointType":       "source",
			"EngineName":         "mysql",
		})
		require.Equal(t, http.StatusOK, srcRec.Code)
		srcArn := parseJSON(t, srcRec)["Endpoint"].(map[string]any)["EndpointArn"].(string)

		tgtRec := doDMS(t, h, "CreateEndpoint", map[string]any{
			"EndpointIdentifier": prefix + "-tgt",
			"EndpointType":       "target",
			"EngineName":         "s3",
		})
		require.Equal(t, http.StatusOK, tgtRec.Code)
		tgtArn := parseJSON(t, tgtRec)["Endpoint"].(map[string]any)["EndpointArn"].(string)

		taskRec := doDMS(t, h, "CreateReplicationTask", map[string]any{
			"ReplicationTaskIdentifier": prefix + "-task",
			"SourceEndpointArn":         srcArn,
			"TargetEndpointArn":         tgtArn,
			"ReplicationInstanceArn":    riArn,
			"MigrationType":             "full-load",
		})
		require.Equal(t, http.StatusOK, taskRec.Code)

		return parseJSON(t, taskRec)["ReplicationTask"].(map[string]any)["ReplicationTaskArn"].(string)
	}

	t.Run("start_nonexistent_task_returns_404", func(t *testing.T) {
		t.Parallel()

		h := newTestDMSHandler()
		rec := doDMS(t, h, "StartReplicationTaskAssessmentRun", map[string]any{
			"ReplicationTaskArn":   "arn:aws:dms:us-east-1:123:task:nonexistent",
			"ServiceAccessRoleArn": "arn:aws:iam::123:role/role",
			"ResultLocationBucket": "my-bucket",
			"AssessmentRunName":    "test-run",
		})
		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("start_stores_run_describable_deletable", func(t *testing.T) {
		t.Parallel()

		h := newTestDMSHandler()
		taskArn := setupTask(t, h, "ar-lifecycle")

		// Start assessment run.
		startRec := doDMS(t, h, "StartReplicationTaskAssessmentRun", map[string]any{
			"ReplicationTaskArn":   taskArn,
			"ServiceAccessRoleArn": "arn:aws:iam::123:role/role",
			"ResultLocationBucket": "my-bucket",
			"AssessmentRunName":    "my-run",
		})
		require.Equal(t, http.StatusOK, startRec.Code)
		runBody := parseJSON(t, startRec)["ReplicationTaskAssessmentRun"].(map[string]any)
		runArn, _ := runBody["ReplicationTaskAssessmentRunArn"].(string)
		assert.NotEmpty(t, runArn, "assessment run ARN must be non-empty")

		// DescribeReplicationTaskAssessmentRuns must return it.
		descRec := doDMS(t, h, "DescribeReplicationTaskAssessmentRuns", map[string]any{})
		require.Equal(t, http.StatusOK, descRec.Code)
		runs := parseJSON(t, descRec)["ReplicationTaskAssessmentRuns"].([]any)
		assert.Len(t, runs, 1)

		// DeleteReplicationTaskAssessmentRun must succeed.
		delRec := doDMS(t, h, "DeleteReplicationTaskAssessmentRun", map[string]any{
			"ReplicationTaskAssessmentRunArn": runArn,
		})
		require.Equal(t, http.StatusOK, delRec.Code)

		// Second delete must return 404.
		del2Rec := doDMS(t, h, "DeleteReplicationTaskAssessmentRun", map[string]any{
			"ReplicationTaskAssessmentRunArn": runArn,
		})
		require.Equal(t, http.StatusNotFound, del2Rec.Code)
	})

	t.Run("cancel_existing_run_succeeds", func(t *testing.T) {
		t.Parallel()

		h := newTestDMSHandler()
		taskArn := setupTask(t, h, "ar-cancel")

		startRec := doDMS(t, h, "StartReplicationTaskAssessmentRun", map[string]any{
			"ReplicationTaskArn":   taskArn,
			"ServiceAccessRoleArn": "arn:aws:iam::123:role/role",
			"ResultLocationBucket": "bucket",
			"AssessmentRunName":    "cancel-run",
		})
		require.Equal(t, http.StatusOK, startRec.Code)
		runBody2 := parseJSON(t, startRec)["ReplicationTaskAssessmentRun"].(map[string]any)
		runArn, _ := runBody2["ReplicationTaskAssessmentRunArn"].(string)

		cancelRec := doDMS(t, h, "CancelReplicationTaskAssessmentRun", map[string]any{
			"ReplicationTaskAssessmentRunArn": runArn,
		})
		require.Equal(t, http.StatusOK, cancelRec.Code)
	})

	t.Run("cancel_nonexistent_run_returns_404", func(t *testing.T) {
		t.Parallel()

		h := newTestDMSHandler()
		rec := doDMS(t, h, "CancelReplicationTaskAssessmentRun", map[string]any{
			"ReplicationTaskAssessmentRunArn": "arn:aws:dms:us-east-1:123:assessment-run:nonexistent",
		})
		require.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestStartReplicationTaskAssessment(t *testing.T) {
	t.Parallel()

	t.Run("not_found_returns_404", func(t *testing.T) {
		t.Parallel()

		h := newTestDMSHandler()
		rec := doDMS(t, h, "StartReplicationTaskAssessment", map[string]any{
			"ReplicationTaskArn": "arn:aws:dms:us-east-1:123:task:nonexistent",
		})
		require.Equal(t, http.StatusNotFound, rec.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "ResourceNotFoundFault", body["__type"])
	})

	t.Run("returns_task_on_success", func(t *testing.T) {
		t.Parallel()

		h := newTestDMSHandler()

		riRec := doDMS(t, h, "CreateReplicationInstance", map[string]any{
			"ReplicationInstanceIdentifier": "assess-ri",
			"ReplicationInstanceClass":      "dms.t3.medium",
		})
		require.Equal(t, http.StatusOK, riRec.Code)
		riArn := parseJSON(t, riRec)["ReplicationInstance"].(map[string]any)["ReplicationInstanceArn"].(string)

		srcRec := doDMS(t, h, "CreateEndpoint", map[string]any{
			"EndpointIdentifier": "assess-src",
			"EndpointType":       "source",
			"EngineName":         "mysql",
		})
		require.Equal(t, http.StatusOK, srcRec.Code)
		srcArn := parseJSON(t, srcRec)["Endpoint"].(map[string]any)["EndpointArn"].(string)

		tgtRec := doDMS(t, h, "CreateEndpoint", map[string]any{
			"EndpointIdentifier": "assess-tgt",
			"EndpointType":       "target",
			"EngineName":         "s3",
		})
		require.Equal(t, http.StatusOK, tgtRec.Code)
		tgtArn := parseJSON(t, tgtRec)["Endpoint"].(map[string]any)["EndpointArn"].(string)

		taskRec := doDMS(t, h, "CreateReplicationTask", map[string]any{
			"ReplicationTaskIdentifier": "assess-task",
			"SourceEndpointArn":         srcArn,
			"TargetEndpointArn":         tgtArn,
			"ReplicationInstanceArn":    riArn,
			"MigrationType":             "full-load",
		})
		require.Equal(t, http.StatusOK, taskRec.Code)
		taskArn := parseJSON(t, taskRec)["ReplicationTask"].(map[string]any)["ReplicationTaskArn"].(string)

		// Real AWS requires the task to be stopped with successful prior
		// connection tests to both endpoints before an assessment can run
		// (api_op_StartReplicationTaskAssessment.go:16-23); a freshly
		// created task is "ready", not "stopped", so drive it through
		// Start/Stop and test both connections first.
		require.Equal(t, http.StatusOK, doDMS(t, h, "TestConnection", map[string]any{
			"ReplicationInstanceArn": riArn,
			"EndpointArn":            srcArn,
		}).Code)
		require.Equal(t, http.StatusOK, doDMS(t, h, "TestConnection", map[string]any{
			"ReplicationInstanceArn": riArn,
			"EndpointArn":            tgtArn,
		}).Code)
		require.Equal(t, http.StatusOK, doDMS(t, h, "StartReplicationTask", map[string]any{
			"ReplicationTaskArn": taskArn,
		}).Code)
		require.Equal(t, http.StatusOK, doDMS(t, h, "StopReplicationTask", map[string]any{
			"ReplicationTaskArn": taskArn,
		}).Code)

		assessRec := doDMS(t, h, "StartReplicationTaskAssessment", map[string]any{
			"ReplicationTaskArn": taskArn,
		})
		require.Equal(t, http.StatusOK, assessRec.Code)

		rt := parseJSON(t, assessRec)["ReplicationTask"].(map[string]any)
		assert.Equal(t, taskArn, rt["ReplicationTaskArn"],
			"StartReplicationTaskAssessment must return the actual task ARN")
		// Status must not be the old hardcoded "test-failed".
		assert.NotEqual(t, "test-failed", rt["Status"],
			"StartReplicationTaskAssessment must not return test-failed as initial status")
	})

	t.Run("rejects_task_not_stopped", func(t *testing.T) {
		t.Parallel()

		h := newTestDMSHandler()
		taskArn, riArn, srcArn, tgtArn := setupAssessmentTaskWithEndpoints(t, h, "notstopped")

		// Both endpoints have successful connections, isolating this case
		// to the task-state guard: task is freshly created ("ready"), not
		// stopped, and AWS requires the task to be stopped
		// (api_op_StartReplicationTaskAssessment.go:16-23).
		require.Equal(t, http.StatusOK, doDMS(t, h, "TestConnection", map[string]any{
			"ReplicationInstanceArn": riArn,
			"EndpointArn":            srcArn,
		}).Code)
		require.Equal(t, http.StatusOK, doDMS(t, h, "TestConnection", map[string]any{
			"ReplicationInstanceArn": riArn,
			"EndpointArn":            tgtArn,
		}).Code)

		rec := doDMS(t, h, "StartReplicationTaskAssessment", map[string]any{
			"ReplicationTaskArn": taskArn,
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "InvalidResourceStateFault", body["__type"])
	})

	t.Run("rejects_task_without_successful_connections", func(t *testing.T) {
		t.Parallel()

		h := newTestDMSHandler()
		taskArn := setupAssessmentTask(t, h, "noconn")

		require.Equal(t, http.StatusOK, doDMS(t, h, "StartReplicationTask", map[string]any{
			"ReplicationTaskArn": taskArn,
		}).Code)
		require.Equal(t, http.StatusOK, doDMS(t, h, "StopReplicationTask", map[string]any{
			"ReplicationTaskArn": taskArn,
		}).Code)

		// Task is stopped, but neither endpoint has a recorded TestConnection.
		rec := doDMS(t, h, "StartReplicationTaskAssessment", map[string]any{
			"ReplicationTaskArn": taskArn,
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "InvalidResourceStateFault", body["__type"])
	})
}

// setupAssessmentTask creates a replication instance, source/target
// endpoints, and a replication task, returning the task's ARN.
func setupAssessmentTask(t *testing.T, h *dms.Handler, prefix string) string {
	t.Helper()

	taskArn, _, _, _ := setupAssessmentTaskWithEndpoints(t, h, prefix)

	return taskArn
}

// setupAssessmentTaskWithEndpoints creates a replication instance,
// source/target endpoints, and a replication task, returning the task's ARN
// alongside the replication instance and endpoint ARNs (needed to drive
// TestConnection in assessment-precondition tests).
func setupAssessmentTaskWithEndpoints(
	t *testing.T, h *dms.Handler, prefix string,
) (string, string, string, string) {
	t.Helper()

	riRec := doDMS(t, h, "CreateReplicationInstance", map[string]any{
		"ReplicationInstanceIdentifier": prefix + "-ri",
		"ReplicationInstanceClass":      "dms.t3.medium",
	})
	require.Equal(t, http.StatusOK, riRec.Code)
	riArn := parseJSON(t, riRec)["ReplicationInstance"].(map[string]any)["ReplicationInstanceArn"].(string)

	srcRec := doDMS(t, h, "CreateEndpoint", map[string]any{
		"EndpointIdentifier": prefix + "-src",
		"EndpointType":       "source",
		"EngineName":         "mysql",
	})
	require.Equal(t, http.StatusOK, srcRec.Code)
	srcArn := parseJSON(t, srcRec)["Endpoint"].(map[string]any)["EndpointArn"].(string)

	tgtRec := doDMS(t, h, "CreateEndpoint", map[string]any{
		"EndpointIdentifier": prefix + "-tgt",
		"EndpointType":       "target",
		"EngineName":         "s3",
	})
	require.Equal(t, http.StatusOK, tgtRec.Code)
	tgtArn := parseJSON(t, tgtRec)["Endpoint"].(map[string]any)["EndpointArn"].(string)

	taskRec := doDMS(t, h, "CreateReplicationTask", map[string]any{
		"ReplicationTaskIdentifier": prefix + "-task",
		"SourceEndpointArn":         srcArn,
		"TargetEndpointArn":         tgtArn,
		"ReplicationInstanceArn":    riArn,
		"MigrationType":             "full-load",
	})
	require.Equal(t, http.StatusOK, taskRec.Code)
	taskArn := parseJSON(t, taskRec)["ReplicationTask"].(map[string]any)["ReplicationTaskArn"].(string)

	return taskArn, riArn, srcArn, tgtArn
}

// TestStartReplicationTaskAssessmentRun_RequiredFields verifies
// ValidationException is returned when any of the four required
// StartReplicationTaskAssessmentRunInput members is missing, and when both
// IncludeOnly and Exclude are set (mutually exclusive per the SDK doc).
func TestStartReplicationTaskAssessmentRun_RequiredFields(t *testing.T) {
	t.Parallel()

	full := map[string]any{
		"ReplicationTaskArn":   "arn:aws:dms:us-east-1:123:task:t",
		"ServiceAccessRoleArn": "arn:aws:iam::123:role/role",
		"ResultLocationBucket": "bucket",
		"AssessmentRunName":    "run",
	}

	cases := []struct {
		body map[string]any
		name string
	}{
		{name: "missing AssessmentRunName", body: withoutKey(full, "AssessmentRunName")},
		{name: "missing ReplicationTaskArn", body: withoutKey(full, "ReplicationTaskArn")},
		{name: "missing ResultLocationBucket", body: withoutKey(full, "ResultLocationBucket")},
		{name: "missing ServiceAccessRoleArn", body: withoutKey(full, "ServiceAccessRoleArn")},
		{
			name: "both IncludeOnly and Exclude set",
			body: mergeMaps(full, map[string]any{
				"IncludeOnly": []string{"test-connection-source"},
				"Exclude":     []string{"test-connection-target"},
			}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestDMSHandler()
			rec := doDMS(t, h, "StartReplicationTaskAssessmentRun", tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func withoutKey(m map[string]any, key string) map[string]any {
	out := make(map[string]any, len(m))
	maps.Copy(out, m)
	delete(out, key)

	return out
}

func mergeMaps(base, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	maps.Copy(out, base)
	maps.Copy(out, extra)

	return out
}

// TestDescribeApplicableIndividualAssessments verifies the op returns a
// non-empty static catalog of individual assessment names (previously
// always an empty list regardless of input).
func TestDescribeApplicableIndividualAssessments(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	rec := doDMS(t, h, "DescribeApplicableIndividualAssessments", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	names := parseJSON(t, rec)["IndividualAssessmentNames"].([]any)
	assert.NotEmpty(t, names, "DescribeApplicableIndividualAssessments must return a non-empty catalog")
}

// TestAssessmentRun_IndividualAssessmentsAndResults verifies that starting
// an assessment run populates real, filterable individual-assessment rows
// (DescribeReplicationTaskIndividualAssessments) and a real assessment
// result (DescribeReplicationTaskAssessmentResults), both of which were
// previously always-empty lists regardless of any StartReplicationTaskAssessmentRun
// call.
func TestAssessmentRun_IndividualAssessmentsAndResults(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	taskArn := setupAssessmentTask(t, h, "ar-ind")

	startRec := doDMS(t, h, "StartReplicationTaskAssessmentRun", map[string]any{
		"ReplicationTaskArn":   taskArn,
		"ServiceAccessRoleArn": "arn:aws:iam::123:role/role",
		"ResultLocationBucket": "bucket",
		"AssessmentRunName":    "ind-run",
		"IncludeOnly":          []string{"test-connection-source", "test-connection-target"},
	})
	require.Equal(t, http.StatusOK, startRec.Code)
	runBody := parseJSON(t, startRec)["ReplicationTaskAssessmentRun"].(map[string]any)
	runArn := runBody["ReplicationTaskAssessmentRunArn"].(string)
	assert.Equal(t, "passed", runBody["Status"])
	stats := runBody["ResultStatistic"].(map[string]any)
	assert.InDelta(t, float64(2), stats["Passed"], 0)

	// Unfiltered: both individual assessments must be visible.
	indRec := doDMS(t, h, "DescribeReplicationTaskIndividualAssessments", map[string]any{})
	require.Equal(t, http.StatusOK, indRec.Code)
	individuals := parseJSON(t, indRec)["ReplicationTaskIndividualAssessments"].([]any)
	require.Len(t, individuals, 2)

	names := make([]string, 0, len(individuals))
	for _, raw := range individuals {
		names = append(names, raw.(map[string]any)["IndividualAssessmentName"].(string))
	}
	assert.ElementsMatch(t, []string{"test-connection-source", "test-connection-target"}, names)

	// Filtered by the run ARN.
	filteredRec := doDMS(t, h, "DescribeReplicationTaskIndividualAssessments", map[string]any{
		"Filters": []map[string]any{
			{"Name": "replication-task-assessment-run-arn", "Values": []string{runArn}},
		},
	})
	require.Equal(t, http.StatusOK, filteredRec.Code)
	filtered := parseJSON(t, filteredRec)["ReplicationTaskIndividualAssessments"].([]any)
	assert.Len(t, filtered, 2)

	// DescribeReplicationTaskAssessmentResults, filtered by task ARN, returns
	// exactly one result and ignores Marker/MaxRecords per the SDK doc.
	resultRec := doDMS(t, h, "DescribeReplicationTaskAssessmentResults", map[string]any{
		"ReplicationTaskArn": taskArn,
	})
	require.Equal(t, http.StatusOK, resultRec.Code)
	results := parseJSON(t, resultRec)["ReplicationTaskAssessmentResults"].([]any)
	require.Len(t, results, 1)
	result0 := results[0].(map[string]any)
	assert.Equal(t, taskArn, result0["ReplicationTaskArn"])
	assert.Equal(t, "passed", result0["AssessmentStatus"])

	// Unfiltered list form also reports the completed run.
	allResultsRec := doDMS(t, h, "DescribeReplicationTaskAssessmentResults", map[string]any{})
	require.Equal(t, http.StatusOK, allResultsRec.Code)
	allResults := parseJSON(t, allResultsRec)["ReplicationTaskAssessmentResults"].([]any)
	require.Len(t, allResults, 1)

	// A task that was never assessed reports no result.
	noneRec := doDMS(t, h, "DescribeReplicationTaskAssessmentResults", map[string]any{
		"ReplicationTaskArn": "arn:aws:dms:us-east-1:123:task:never-assessed",
	})
	require.Equal(t, http.StatusOK, noneRec.Code)
	assert.Empty(t, parseJSON(t, noneRec)["ReplicationTaskAssessmentResults"])
}

func TestHandler_CancelReplicationTaskAssessmentRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    map[string]any
		name     string
		wantCode int
	}{
		{
			name:     "missing_arn",
			wantCode: http.StatusBadRequest,
			input:    map[string]any{},
		},
		{
			name:     "not_found",
			wantCode: http.StatusNotFound,
			input: map[string]any{
				"ReplicationTaskAssessmentRunArn": "arn:aws:dms:us-east-1:123:assessment-run:nonexistent",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestDMSHandler()
			rec := doDMS(t, h, "CancelReplicationTaskAssessmentRun", tt.input)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}
