package mediaconvert_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/mediaconvert"
)

func TestMediaConvert_Queue_CRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		queueName  string
		wantInBody string
		wantStatus int
	}{
		{
			name:       "create_queue",
			queueName:  "my-queue",
			wantStatus: http.StatusCreated,
			wantInBody: "my-queue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// Create
			rec := doRequest(t, h, http.MethodPost, "/2017-08-29/queues", map[string]any{
				"name":        tt.queueName,
				"description": "test queue",
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantInBody)
		})
	}
}

func TestMediaConvert_Queue_FullLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	queueName := "test-queue"

	// Create queue
	rec := doRequest(t, h, http.MethodPost, "/2017-08-29/queues", map[string]any{
		"name":        queueName,
		"description": "initial description",
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&createResp))
	queueData, ok := createResp["queue"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, queueName, queueData["name"])
	assert.Equal(t, "ACTIVE", queueData["status"])
	assert.Equal(t, "ON_DEMAND", queueData["pricingPlan"])
	assert.Equal(t, "CUSTOM", queueData["type"])
	assert.NotEmpty(t, queueData["arn"])

	// Get queue
	rec = doRequest(t, h, http.MethodGet, "/2017-08-29/queues/"+queueName, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), queueName)

	// List queues
	rec = doRequest(t, h, http.MethodGet, "/2017-08-29/queues", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), queueName)

	// Update queue
	rec = doRequest(t, h, http.MethodPut, "/2017-08-29/queues/"+queueName, map[string]any{
		"description": "updated description",
		"status":      "PAUSED",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var updateResp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&updateResp))
	updatedQueue, ok := updateResp["queue"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "PAUSED", updatedQueue["status"])

	// Delete queue
	rec = doRequest(t, h, http.MethodDelete, "/2017-08-29/queues/"+queueName, nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify deleted
	rec = doRequest(t, h, http.MethodGet, "/2017-08-29/queues/"+queueName, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMediaConvert_Queue_DuplicateCreate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, http.MethodPost, "/2017-08-29/queues", map[string]any{"name": "dup-queue"})

	rec := doRequest(t, h, http.MethodPost, "/2017-08-29/queues", map[string]any{"name": "dup-queue"})
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestMediaConvert_Queue_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodGet, "/2017-08-29/queues/nonexistent", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMediaConvert_Queue_MissingName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/2017-08-29/queues", map[string]any{
		"description": "no name here",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestMediaConvert_ListQueues_Empty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodGet, "/2017-08-29/queues", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	queues, ok := out["queues"].([]any)
	require.True(t, ok)
	assert.Empty(t, queues)
}

func TestMediaConvert_DeleteQueue_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodDelete, "/2017-08-29/queues/nonexistent", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMediaConvert_UpdateQueue_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPut, "/2017-08-29/queues/nonexistent", map[string]any{
		"status": "PAUSED",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestMediaConvert_UpdateQueue_ConcurrentJobsAndReservationPlanSettings verifies
// that UpdateQueue applies concurrentJobs and reservationPlanSettings -- real
// UpdateQueueInput members that were previously silently dropped.
func TestMediaConvert_UpdateQueue_ConcurrentJobsAndReservationPlanSettings(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/2017-08-29/queues", map[string]any{
		"name": "update-queue-fields",
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	rec = doRequest(t, h, http.MethodPut, "/2017-08-29/queues/update-queue-fields", map[string]any{
		"concurrentJobs": 7,
		"reservationPlanSettings": map[string]any{
			"commitment":    "ONE_YEAR",
			"renewalType":   "AUTO_RENEW",
			"reservedSlots": 4,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	queueData := out["queue"].(map[string]any)
	assert.InDelta(t, float64(7), queueData["concurrentJobs"], 0)

	rp, ok := queueData["reservationPlan"].(map[string]any)
	require.True(t, ok, "reservationPlan should be in response")
	assert.InDelta(t, float64(4), rp["reservedSlots"], 0)
	assert.Equal(t, "ONE_YEAR", rp["commitment"])
}

// TestMediaConvert_CreateQueue_MaximumConcurrentFeeds verifies
// CreateQueueInput.maximumConcurrentFeeds (added to the real API after
// createQueueInput's field list was written, see gopherstack-gt9o) is stored
// and echoed back exactly as supplied, and is absent from the raw wire body
// -- not present as null/0 -- when the caller never sent it.
func TestMediaConvert_CreateQueue_MaximumConcurrentFeeds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check func(t *testing.T, queue map[string]any)
		body  map[string]any
		name  string
	}{
		{
			name: "supplied",
			body: map[string]any{
				"name":                   "mcf-queue-supplied",
				"maximumConcurrentFeeds": 5,
			},
			check: func(t *testing.T, queue map[string]any) {
				t.Helper()
				assert.InDelta(t, float64(5), queue["maximumConcurrentFeeds"], 0)
			},
		},
		{
			name: "absent",
			body: map[string]any{
				"name": "mcf-queue-absent",
			},
			check: func(t *testing.T, queue map[string]any) {
				t.Helper()

				_, ok := queue["maximumConcurrentFeeds"]
				assert.False(t, ok, "maximumConcurrentFeeds must be absent, not null/0, when unset")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPost, "/2017-08-29/queues", tt.body)
			require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

			var createResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
			createQueue, ok := createResp["queue"].(map[string]any)
			require.True(t, ok)
			tt.check(t, createQueue)

			name, _ := tt.body["name"].(string)
			rec = doRequest(t, h, http.MethodGet, "/2017-08-29/queues/"+name, nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var getResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
			getQueue, ok := getResp["queue"].(map[string]any)
			require.True(t, ok)
			tt.check(t, getQueue)
		})
	}
}

// TestMediaConvert_UpdateQueue_MaximumConcurrentFeeds verifies
// UpdateQueueInput.maximumConcurrentFeeds is applied, not silently dropped.
func TestMediaConvert_UpdateQueue_MaximumConcurrentFeeds(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/2017-08-29/queues", map[string]any{
		"name": "update-mcf-queue",
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	rec = doRequest(t, h, http.MethodPut, "/2017-08-29/queues/update-mcf-queue", map[string]any{
		"maximumConcurrentFeeds": 9,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	queueData, ok := out["queue"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, float64(9), queueData["maximumConcurrentFeeds"], 0)
}

func TestMediaConvert_TagLeakOnDeleteQueue(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)

	q, err := b.CreateQueue("my-queue", "", "", "", nil)
	require.NoError(t, err)

	b.TagResource(q.Arn, map[string]string{"env": "test"})
	assert.Equal(t, "test", b.GetTags(q.Arn)["env"])

	require.NoError(t, b.DeleteQueue("my-queue"))

	// Tags must be gone after deletion.
	assert.Empty(t, b.GetTags(q.Arn))
}

// TestCreateQueue_WithTags verifies tags are stored at creation time.
func TestCreateQueue_WithTags(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/2017-08-29/queues", map[string]any{
		"name": "tagged-queue",
		"tags": map[string]string{"env": "prod", "team": "infra"},
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	queue := resp["queue"].(map[string]any)
	tags := queue["tags"].(map[string]any)
	assert.Equal(t, "prod", tags["env"])
	assert.Equal(t, "infra", tags["team"])
}

// TestUpdateQueue_InvalidStatus verifies bad status returns 400.
func TestUpdateQueue_InvalidStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     string
		wantStatus int
	}{
		{name: "invalid_status", status: "RUNNING", wantStatus: http.StatusBadRequest},
		{name: "active_status", status: "ACTIVE", wantStatus: http.StatusOK},
		{name: "paused_status", status: "PAUSED", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, http.MethodPost, "/2017-08-29/queues", map[string]any{"name": "test-q"})
			rec := doRequest(t, h, http.MethodPut, "/2017-08-29/queues/test-q", map[string]any{"status": tt.status})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestListQueues_SortedByName verifies list is sorted.
func TestListQueues_SortedByName(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)

	for _, name := range []string{"c-queue", "a-queue", "b-queue"} {
		_, err := b.CreateQueue(name, "", "", "", nil)
		require.NoError(t, err)
	}

	queues := b.ListQueues()
	require.Len(t, queues, 3)
	assert.Equal(t, "a-queue", queues[0].Name)
	assert.Equal(t, "b-queue", queues[1].Name)
	assert.Equal(t, "c-queue", queues[2].Name)
}

// TestCreateQueue_TypeIsCustom verifies queue type is always CUSTOM.
func TestCreateQueue_TypeIsCustom(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	q, err := b.CreateQueue("type-queue", "", "", "", nil)
	require.NoError(t, err)
	assert.Equal(t, "CUSTOM", q.Type)
}

// TestCreateQueue_DefaultStatusActive verifies default queue status is ACTIVE.
func TestCreateQueue_DefaultStatusActive(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	q, err := b.CreateQueue("default-status-queue", "", "", "", nil)
	require.NoError(t, err)
	assert.Equal(t, "ACTIVE", q.Status)
}

// TestCreateQueue_DefaultPricingPlanOnDemand verifies default pricing plan is ON_DEMAND.
func TestCreateQueue_DefaultPricingPlanOnDemand(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	q, err := b.CreateQueue("default-pricing-queue", "", "", "", nil)
	require.NoError(t, err)
	assert.Equal(t, "ON_DEMAND", q.PricingPlan)
}

// TestQueue_JobCounts verifies ProgressingJobsCount/SubmittedJobsCount are computed.
func TestQueue_JobCounts(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	_, err := b.CreateQueue("test-q", "", "", "", nil)
	require.NoError(t, err)

	j1, err := b.CreateJob("arn:aws:iam::123:role/r", "test-q", "", nil, nil, nil, "")
	require.NoError(t, err)

	_, err = b.CreateJob("arn:aws:iam::123:role/r", "test-q", "", nil, nil, nil, "")
	require.NoError(t, err)

	err = b.CancelJob(j1.ID)
	require.NoError(t, err)

	q, err := b.GetQueue("test-q")
	require.NoError(t, err)

	assert.Equal(t, 1, q.SubmittedJobsCount)
	assert.Equal(t, 0, q.ProgressingJobsCount)
}

// TestListQueues_JobCountsIncluded verifies job counts appear in ListQueues.
func TestListQueues_JobCountsIncluded(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	_, err := b.CreateQueue("list-q", "", "", "", nil)
	require.NoError(t, err)

	_, err = b.CreateJob("arn:aws:iam::123:role/r", "list-q", "", nil, nil, nil, "")
	require.NoError(t, err)

	queues := b.ListQueues()
	require.Len(t, queues, 1)
	assert.Equal(t, 1, queues[0].SubmittedJobsCount)
}

// TestListQueues_JobCountsViaHTTP verifies HTTP /queues includes counts.
func TestListQueues_JobCountsViaHTTP(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	h := mediaconvert.NewHandler(b)

	_, err := b.CreateQueue("count-q", "", "", "", nil)
	require.NoError(t, err)

	_, err = b.CreateJob("arn:aws:iam::123:role/r", "count-q", "", nil, nil, nil, "")
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodGet, "/2017-08-29/queues", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	queues, ok := out["queues"].([]any)
	require.True(t, ok)
	require.Len(t, queues, 1)
	q := queues[0].(map[string]any)
	assert.InDelta(t, float64(1), q["submittedJobsCount"], 0)
}

// TestCreateQueue_ReservationPlan verifies reservation plan stored at creation.
func TestCreateQueue_ReservationPlan(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	rp := &mediaconvert.ReservationPlan{
		ReservedSlots: 5,
		Status:        "ACTIVE",
		Commitment:    "ONE_YEAR",
		RenewalType:   "AUTO_RENEW",
	}

	q, err := b.CreateQueueFull("rp-queue", "", "", "", nil, nil, rp)
	require.NoError(t, err)
	require.NotNil(t, q.ReservationPlan)
	assert.Equal(t, 5, q.ReservationPlan.ReservedSlots)
	assert.Equal(t, "ACTIVE", q.ReservationPlan.Status)
}

// TestCreateQueue_ReservationPlanViaHTTP verifies JSON parsing. The real
// CreateQueueInput wire field is "reservationPlanSettings" (the response
// Queue resource echoes it back as "reservationPlan" -- request and
// response field names differ).
func TestCreateQueue_ReservationPlanViaHTTP(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/2017-08-29/queues", map[string]any{
		"name": "rp-http-queue",
		"reservationPlanSettings": map[string]any{
			"reservedSlots": 3,
			"status":        "ACTIVE",
		},
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var out map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	queueData := out["queue"].(map[string]any)
	rp, ok := queueData["reservationPlan"].(map[string]any)
	require.True(t, ok, "reservationPlan should be in response")
	assert.InDelta(t, float64(3), rp["reservedSlots"], 0)
}

// TestQueue_NoLegacyReservationPlanNameField verifies old field removed.
func TestQueue_NoLegacyReservationPlanNameField(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	q, err := b.CreateQueue("no-rp-name-q", "", "", "", nil)
	require.NoError(t, err)

	h := mediaconvert.NewHandler(b)
	rec := doRequest(t, h, http.MethodGet, "/2017-08-29/queues/"+q.Name, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	queueData := out["queue"].(map[string]any)
	_, hasOldField := queueData["reservationPlanName"]
	assert.False(t, hasOldField, "legacy reservationPlanName field should not be present")
}

// TestCreateQueue_ConcurrentJobs verifies field stored at creation.
func TestCreateQueue_ConcurrentJobs(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	limit := 8
	q, err := b.CreateQueueFull("cj-queue", "", "", "", nil, &limit, nil)
	require.NoError(t, err)
	require.NotNil(t, q.ConcurrentJobs)
	assert.Equal(t, 8, *q.ConcurrentJobs)
}

// TestCreateQueue_ConcurrentJobsViaHTTP verifies JSON round-trip.
func TestCreateQueue_ConcurrentJobsViaHTTP(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/2017-08-29/queues", map[string]any{
		"name":           "cj-http-queue",
		"concurrentJobs": 4,
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var out map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	queueData := out["queue"].(map[string]any)
	assert.InDelta(t, float64(4), queueData["concurrentJobs"], 0)
}

// TestCreateQueue_ConcurrentJobsZeroDistinctFromUnset proves gopherstack-7bxb:
// a client that never sends concurrentJobs and a client that explicitly sends
// concurrentJobs:0 must not collapse to the same stored/emitted value. Real
// CreateQueueInput.ConcurrentJobs is *int32 (api_op_CreateQueue.go:42), so
// AWS itself distinguishes absent from an explicit zero.
func TestCreateQueue_ConcurrentJobsZeroDistinctFromUnset(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	unset := doRequest(t, h, http.MethodPost, "/2017-08-29/queues", map[string]any{
		"name": "cj-unset-queue",
	})
	require.Equal(t, http.StatusCreated, unset.Code)

	var unsetOut map[string]any
	require.NoError(t, json.NewDecoder(unset.Body).Decode(&unsetOut))
	unsetQueue := unsetOut["queue"].(map[string]any)
	_, hasField := unsetQueue["concurrentJobs"]
	assert.False(t, hasField, "concurrentJobs must be omitted entirely when the client never sent it")

	explicitZero := doRequest(t, h, http.MethodPost, "/2017-08-29/queues", map[string]any{
		"name":           "cj-zero-queue",
		"concurrentJobs": 0,
	})
	require.Equal(t, http.StatusCreated, explicitZero.Code)

	var zeroOut map[string]any
	require.NoError(t, json.NewDecoder(explicitZero.Body).Decode(&zeroOut))
	zeroQueue := zeroOut["queue"].(map[string]any)
	zeroVal, hasZeroField := zeroQueue["concurrentJobs"]
	require.True(t, hasZeroField, "an explicit concurrentJobs:0 must round-trip as a present field, not be dropped")
	assert.InDelta(t, float64(0), zeroVal, 0)
}

// TestCreateQueue_NilReservationPlanByDefault verifies nil is fine.
func TestCreateQueue_NilReservationPlanByDefault(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	q, err := b.CreateQueue("no-rp", "", "", "", nil)
	require.NoError(t, err)
	assert.Nil(t, q.ReservationPlan)
}

// TestQueueCounters_Accurate verifies counters during lifecycle.
func TestQueueCounters_Accurate(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	q, err := b.CreateQueue("lifecycle-q", "", "", "", nil)
	require.NoError(t, err)

	// Start: 0/0
	assert.Equal(t, 0, mediaconvert.QueueCounterSubmitted(b, q.Arn))
	assert.Equal(t, 0, mediaconvert.QueueCounterProgressing(b, q.Arn))

	// Create 2 jobs
	j1, err := b.CreateJob("arn:aws:iam::123:role/r", "lifecycle-q", "", nil, nil, nil, "")
	require.NoError(t, err)

	_, err = b.CreateJob("arn:aws:iam::123:role/r", "lifecycle-q", "", nil, nil, nil, "")
	require.NoError(t, err)

	// 2 submitted
	assert.Equal(t, 2, mediaconvert.QueueCounterSubmitted(b, q.Arn))

	// Advance once: both go to PROGRESSING
	b.AdvanceJobPhase()
	assert.Equal(t, 0, mediaconvert.QueueCounterSubmitted(b, q.Arn))
	assert.Equal(t, 2, mediaconvert.QueueCounterProgressing(b, q.Arn))

	// Cancel j1 (which is now PROGRESSING)
	require.NoError(t, b.CancelJob(j1.ID))
	assert.Equal(t, 1, mediaconvert.QueueCounterProgressing(b, q.Arn))
}

// TestGetQueue_UsesCounters verifies GetQueue reads counters.
func TestGetQueue_UsesCounters(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	_, err := b.CreateQueue("counter-q2", "", "", "", nil)
	require.NoError(t, err)

	_, err = b.CreateJob("arn:aws:iam::123:role/r", "counter-q2", "", nil, nil, nil, "")
	require.NoError(t, err)

	q, err := b.GetQueue("counter-q2")
	require.NoError(t, err)
	assert.Equal(t, 1, q.SubmittedJobsCount)
	assert.Equal(t, 0, q.ProgressingJobsCount)
}

// TestListQueues_UsesCounters verifies ListQueues reads counters.
func TestListQueues_UsesCounters(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)
	_, err := b.CreateQueue("list-counter-q", "", "", "", nil)
	require.NoError(t, err)

	_, err = b.CreateJob("arn:aws:iam::123:role/r", "list-counter-q", "", nil, nil, nil, "")
	require.NoError(t, err)

	queues := b.ListQueues()
	require.Len(t, queues, 1)
	assert.Equal(t, 1, queues[0].SubmittedJobsCount)
}

// TestResolveQueueByARN verifies a job can be created by referencing a queue by ARN.
func TestResolveQueueByARN(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)

	q, err := b.CreateQueue("my-queue", "", "", "", nil)
	require.NoError(t, err)

	h := mediaconvert.NewHandler(b)

	// Create job using queue ARN — triggers resolveQueueLocked with ARN path.
	rec := doRequest(t, h, http.MethodPost, "/2017-08-29/jobs", map[string]any{
		"role":     "arn:aws:iam::" + testAccountID + ":role/Role",
		"queue":    q.Arn,
		"settings": map[string]any{},
	})
	assert.Equal(t, http.StatusCreated, rec.Code)

	// Queue ARN should resolve to correct queue.
	rec2 := doRequest(t, h, http.MethodGet, "/2017-08-29/queues/my-queue", nil)
	assert.Equal(t, http.StatusOK, rec2.Code)
}

// TestQueuesByArnIndex_SurvivesDelete verifies the byARN index no longer
// resolves a queue after it has been deleted.
func TestQueuesByArnIndex_SurvivesDelete(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)

	q, err := b.CreateQueue("del-queue", "", "", "", nil)
	require.NoError(t, err)

	err = b.DeleteQueue("del-queue")
	require.NoError(t, err)

	// After delete, creating job with deleted queue ARN should fail (queue not found).
	h := mediaconvert.NewHandler(b)
	rec := doRequest(t, h, http.MethodPost, "/2017-08-29/jobs", map[string]any{
		"role":     "arn:aws:iam::" + testAccountID + ":role/Role",
		"queue":    q.Arn,
		"settings": map[string]any{},
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestQueuesByArn_AddQueueInternal verifies a queue seeded directly (bypassing
// CreateQueue) is still resolvable by ARN.
func TestQueuesByArn_AddQueueInternal(t *testing.T) {
	t.Parallel()

	b := mediaconvert.NewInMemoryBackend(testAccountID, testRegion)

	q := &mediaconvert.Queue{
		Name: "seeded-queue",
		Arn:  "arn:aws:mediaconvert:" + testRegion + ":" + testAccountID + ":queues/seeded-queue",
	}
	b.AddQueueInternal(q)

	// Job using the seeded queue's ARN should resolve correctly.
	h := mediaconvert.NewHandler(b)
	rec := doRequest(t, h, http.MethodPost, "/2017-08-29/jobs", map[string]any{
		"role":     "arn:aws:iam::" + testAccountID + ":role/Role",
		"queue":    q.Arn,
		"settings": map[string]any{},
	})
	assert.Equal(t, http.StatusCreated, rec.Code)
}
