package pinpoint_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	pinpointsdk "github.com/aws/aws-sdk-go-v2/service/pinpoint"
	pinpointtypes "github.com/aws/aws-sdk-go-v2/service/pinpoint/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJourneyFullDTO_Create(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body              map[string]any
		name              string
		wantState         string
		wantRefreshFreq   string
		wantStatus        int
		wantHasActivities bool
		wantHasStartCond  bool
		wantHasSchedule   bool
		wantHasLimits     bool
	}{
		{
			name:       "minimal_journey",
			body:       map[string]any{"Name": "journey"},
			wantStatus: http.StatusCreated,
			wantState:  "DRAFT",
		},
		{
			name: "journey_with_activities",
			body: map[string]any{
				"Name": "active-journey",
				"Activities": map[string]any{
					"activity-1": map[string]any{
						"Wait": map[string]any{
							"WaitTime":     map[string]any{"WaitFor": "PT1H"},
							"NextActivity": "activity-2",
						},
					},
					"activity-2": map[string]any{
						"EMAIL": map[string]any{
							"MessageConfig": map[string]any{"FromAddress": "noreply@example.com"},
							"NextActivity":  "",
						},
					},
				},
				"StartActivity": "activity-1",
			},
			wantStatus:        http.StatusCreated,
			wantHasActivities: true,
		},
		{
			name: "journey_with_start_condition",
			body: map[string]any{
				"Name": "conditional-start",
				"StartCondition": map[string]any{
					"Description": "Start when event fires",
					"EventStartCondition": map[string]any{
						"EventFilter": map[string]any{
							"Dimensions": map[string]any{
								"EventType": map[string]any{
									"DimensionType": "INCLUSIVE",
									"Values":        []any{"_session.start"},
								},
							},
							"FilterType": "SYSTEM",
						},
					},
				},
			},
			wantStatus:       http.StatusCreated,
			wantHasStartCond: true,
		},
		{
			name: "journey_with_schedule",
			body: map[string]any{
				"Name": "scheduled-journey",
				"Schedule": map[string]any{
					"StartTime": "2026-01-01T00:00:00Z",
					"EndTime":   "2026-12-31T23:59:59Z",
					"Timezone":  "UTC",
				},
			},
			wantStatus:      http.StatusCreated,
			wantHasSchedule: true,
		},
		{
			name: "journey_with_limits",
			body: map[string]any{
				"Name": "limited-journey",
				"Limits": map[string]any{
					"DailyCap":                100,
					"EndpointReentryCap":      3,
					"EndpointReentryInterval": "P1D",
					"MessagesPerSecond":       50,
				},
			},
			wantStatus:    http.StatusCreated,
			wantHasLimits: true,
		},
		{
			name: "journey_with_refresh_frequency",
			body: map[string]any{
				"Name":             "refresh-journey",
				"RefreshFrequency": "PT1H",
			},
			wantStatus:      http.StatusCreated,
			wantRefreshFreq: "PT1H",
		},
		{
			name: "journey_with_quiet_time",
			body: map[string]any{
				"Name": "quiet-journey",
				"QuietTime": map[string]any{
					"Start": "22:00",
					"End":   "06:00",
				},
				"WaitForQuietTime": true,
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "journey_with_local_time",
			body: map[string]any{
				"Name":      "local-time-journey",
				"LocalTime": true,
			},
			wantStatus: http.StatusCreated,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHandlerForTest(t)

			appRec := doPinpointRequest(t, h, http.MethodPost, "/v1/apps", map[string]any{"Name": "app"})
			require.Equal(t, http.StatusCreated, appRec.Code)
			var appResp map[string]any
			require.NoError(t, json.Unmarshal(appRec.Body.Bytes(), &appResp))
			appID := appResp["Id"].(string)

			rec := doPinpointRequest(t, h, http.MethodPost, "/v1/apps/"+appID+"/journeys", tc.body)
			assert.Equal(t, tc.wantStatus, rec.Code)

			if rec.Code != http.StatusCreated {
				return
			}

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			assert.NotEmpty(t, resp["Id"])
			assert.Equal(t, appID, resp["ApplicationId"])

			wantState := tc.wantState
			if wantState == "" {
				wantState = "DRAFT"
			}

			assert.Equal(t, wantState, resp["State"])

			if tc.wantHasActivities {
				assert.NotNil(t, resp["Activities"])
			}

			if tc.wantHasStartCond {
				assert.NotNil(t, resp["StartCondition"])
			}

			if tc.wantHasSchedule {
				assert.NotNil(t, resp["Schedule"])
			}

			if tc.wantHasLimits {
				assert.NotNil(t, resp["Limits"])
			}

			if tc.wantRefreshFreq != "" {
				assert.Equal(t, tc.wantRefreshFreq, resp["RefreshFrequency"])
			}
		})
	}
}

func TestJourneyStateValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		fromState        string
		toState          string
		setupTransitions []string
		wantStatus       int
	}{
		{
			name:       "draft_to_active",
			fromState:  "DRAFT",
			toState:    "ACTIVE",
			wantStatus: http.StatusOK,
		},
		{
			name:       "draft_to_cancelled",
			fromState:  "DRAFT",
			toState:    "CANCELLED",
			wantStatus: http.StatusOK,
		},
		{
			name:       "draft_to_paused_invalid",
			fromState:  "DRAFT",
			toState:    "PAUSED",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "draft_to_completed_invalid",
			fromState:  "DRAFT",
			toState:    "COMPLETED",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "active_to_paused",
			fromState:  "DRAFT",
			toState:    "PAUSED",
			wantStatus: http.StatusBadRequest, // from DRAFT directly
		},
		{
			name:             "active_to_cancelled",
			setupTransitions: []string{"ACTIVE"},
			toState:          "CANCELLED",
			wantStatus:       http.StatusOK,
		},
		{
			name:             "active_to_paused_valid",
			setupTransitions: []string{"ACTIVE"},
			toState:          "PAUSED",
			wantStatus:       http.StatusOK,
		},
		{
			name:             "active_to_completed",
			setupTransitions: []string{"ACTIVE"},
			toState:          "COMPLETED",
			wantStatus:       http.StatusOK,
		},
		{
			name:             "paused_to_active",
			setupTransitions: []string{"ACTIVE", "PAUSED"},
			toState:          "ACTIVE",
			wantStatus:       http.StatusOK,
		},
		{
			name:             "paused_to_cancelled",
			setupTransitions: []string{"ACTIVE", "PAUSED"},
			toState:          "CANCELLED",
			wantStatus:       http.StatusOK,
		},
		{
			name:             "cancelled_no_transitions",
			setupTransitions: []string{"CANCELLED"},
			toState:          "ACTIVE",
			wantStatus:       http.StatusBadRequest,
		},
		{
			name:             "completed_no_transitions",
			setupTransitions: []string{"ACTIVE", "COMPLETED"},
			toState:          "PAUSED",
			wantStatus:       http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHandlerForTest(t)

			appRec := doPinpointRequest(t, h, http.MethodPost, "/v1/apps", map[string]any{"Name": "app"})
			require.Equal(t, http.StatusCreated, appRec.Code)
			var appResp map[string]any
			require.NoError(t, json.Unmarshal(appRec.Body.Bytes(), &appResp))
			appID := appResp["Id"].(string)

			createRec := doPinpointRequest(t, h, http.MethodPost, "/v1/apps/"+appID+"/journeys",
				map[string]any{"Name": "j"})
			require.Equal(t, http.StatusCreated, createRec.Code)
			var created map[string]any
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &created))
			journeyID := created["Id"].(string)

			for _, state := range tc.setupTransitions {
				r := doPinpointRequest(t, h, http.MethodPut,
					"/v1/apps/"+appID+"/journeys/"+journeyID+"/state",
					map[string]any{"State": state})
				require.Equal(t, http.StatusOK, r.Code, "setup transition to %s failed: %s", state, r.Body.String())
			}

			rec := doPinpointRequest(t, h, http.MethodPut,
				"/v1/apps/"+appID+"/journeys/"+journeyID+"/state",
				map[string]any{"State": tc.toState})
			assert.Equal(t, tc.wantStatus, rec.Code, "transition to %s: %s", tc.toState, rec.Body.String())
		})
	}
}

func TestJourneyUpdate_BlockedWhenActive(t *testing.T) {
	t.Parallel()

	h := newHandlerForTest(t)

	appRec := doPinpointRequest(t, h, http.MethodPost, "/v1/apps", map[string]any{"Name": "app"})
	require.Equal(t, http.StatusCreated, appRec.Code)
	var appResp map[string]any
	require.NoError(t, json.Unmarshal(appRec.Body.Bytes(), &appResp))
	appID := appResp["Id"].(string)

	createRec := doPinpointRequest(t, h, http.MethodPost, "/v1/apps/"+appID+"/journeys",
		map[string]any{"Name": "j"})
	require.Equal(t, http.StatusCreated, createRec.Code)
	var created map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &created))
	journeyID := created["Id"].(string)

	// Activate the journey.
	r := doPinpointRequest(t, h, http.MethodPut,
		"/v1/apps/"+appID+"/journeys/"+journeyID+"/state",
		map[string]any{"State": "ACTIVE"})
	require.Equal(t, http.StatusOK, r.Code)

	// UpdateJourney (activities mutation) must be rejected: the request is
	// well-formed but conflicts with the journey's current (ACTIVE) state,
	// so AWS models this as ConflictException, not BadRequestException.
	updateRec := doPinpointRequest(t, h, http.MethodPut,
		"/v1/apps/"+appID+"/journeys/"+journeyID,
		map[string]any{
			"Name":       "mutated",
			"Activities": map[string]any{"x": map[string]any{}},
		})
	require.Equal(t, http.StatusConflict, updateRec.Code, updateRec.Body.String())

	var errResp map[string]string
	require.NoError(t, json.Unmarshal(updateRec.Body.Bytes(), &errResp))
	assert.Equal(t, "ConflictException", errResp["__type"])
}

// TestJourneyUpdate_BlockedWhenActive_RealClient drives the real
// aws-sdk-go-v2 pinpoint client through UpdateJourney on an ACTIVE journey
// and confirms errors.As unwraps to *types.ConflictException, not
// *types.BadRequestException. UpdateJourney's deserializeOpErrorUpdateJourney
// switch (pinpoint@v1.42.4 deserializers.go) declares both, and only
// ConflictException matches "conflict with the current state of the
// specified resource" (botocore pinpoint/2016-12-01/service-2.json), which
// is what an ACTIVE journey rejecting a structural edit actually is.
func TestJourneyUpdate_BlockedWhenActive_RealClient(t *testing.T) {
	t.Parallel()

	h := newHandlerForTest(t)
	client := newTestPinpointClient(t, h)

	appOut, err := client.CreateApp(t.Context(), &pinpointsdk.CreateAppInput{
		CreateApplicationRequest: &pinpointtypes.CreateApplicationRequest{Name: aws.String("journey-active-app")},
	})
	require.NoError(t, err)
	appID := aws.ToString(appOut.ApplicationResponse.Id)

	journeyOut, err := client.CreateJourney(t.Context(), &pinpointsdk.CreateJourneyInput{
		ApplicationId:       aws.String(appID),
		WriteJourneyRequest: &pinpointtypes.WriteJourneyRequest{Name: aws.String("active-journey")},
	})
	require.NoError(t, err)
	journeyID := journeyOut.JourneyResponse.Id

	_, err = client.UpdateJourneyState(t.Context(), &pinpointsdk.UpdateJourneyStateInput{
		ApplicationId:       aws.String(appID),
		JourneyId:           journeyID,
		JourneyStateRequest: &pinpointtypes.JourneyStateRequest{State: pinpointtypes.StateActive},
	})
	require.NoError(t, err)

	_, err = client.UpdateJourney(t.Context(), &pinpointsdk.UpdateJourneyInput{
		ApplicationId:       aws.String(appID),
		JourneyId:           journeyID,
		WriteJourneyRequest: &pinpointtypes.WriteJourneyRequest{Name: aws.String("mutated")},
	})
	require.Error(t, err)

	var badReq *pinpointtypes.BadRequestException
	require.NotErrorAs(t, err, &badReq,
		"UpdateJourney on an ACTIVE journey must not surface BadRequestException: %v", err)

	var conflict *pinpointtypes.ConflictException
	require.ErrorAs(t, err, &conflict,
		"expected a real ConflictException from the SDK deserializer, got: %v", err)
}

// ──────────────────────────────────────────────────
// Endpoint: full shape with Demographic, Location, Metrics, Status
// ──────────────────────────────────────────────────

func TestJourneyUpdate_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		updateBody map[string]any
		checkField string
	}{
		{
			name: "update_activities",
			updateBody: map[string]any{
				"Activities": map[string]any{
					"step-1": map[string]any{
						"EMAIL": map[string]any{
							"MessageConfig": map[string]any{},
							"NextActivity":  "",
						},
					},
				},
				"StartActivity": "step-1",
			},
			checkField: "Activities",
		},
		{
			name: "update_schedule",
			updateBody: map[string]any{
				"Schedule": map[string]any{
					"StartTime": "2026-06-01T00:00:00Z",
					"Timezone":  "UTC",
				},
			},
			checkField: "Schedule",
		},
		{
			name: "update_limits",
			updateBody: map[string]any{
				"Limits": map[string]any{
					"DailyCap":          200,
					"MessagesPerSecond": 100,
				},
			},
			checkField: "Limits",
		},
		{
			name: "update_quiet_time",
			updateBody: map[string]any{
				"QuietTime": map[string]any{
					"Start": "23:00",
					"End":   "07:00",
				},
				"WaitForQuietTime": true,
			},
			checkField: "QuietTime",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHandlerForTest(t)

			appRec := doPinpointRequest(t, h, http.MethodPost, "/v1/apps", map[string]any{"Name": "app"})
			require.Equal(t, http.StatusCreated, appRec.Code)
			var appResp map[string]any
			require.NoError(t, json.Unmarshal(appRec.Body.Bytes(), &appResp))
			appID := appResp["Id"].(string)

			createRec := doPinpointRequest(t, h, http.MethodPost, "/v1/apps/"+appID+"/journeys",
				map[string]any{"Name": "j"})
			require.Equal(t, http.StatusCreated, createRec.Code)
			var created map[string]any
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &created))
			journeyID := created["Id"].(string)

			rec := doPinpointRequest(t, h, http.MethodPut,
				"/v1/apps/"+appID+"/journeys/"+journeyID, tc.updateBody)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			assert.NotNil(t, resp[tc.checkField], "%s field must be non-nil after update", tc.checkField)
		})
	}
}

// ──────────────────────────────────────────────────
// Campaign + Segment: GetVersions return correct count
// ──────────────────────────────────────────────────

func TestJourney_MultipleActivities_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newHandlerForTest(t)

	appRec := doPinpointRequest(t, h, http.MethodPost, "/v1/apps", map[string]any{"Name": "app"})
	require.Equal(t, http.StatusCreated, appRec.Code)
	var appResp map[string]any
	require.NoError(t, json.Unmarshal(appRec.Body.Bytes(), &appResp))
	appID := appResp["Id"].(string)

	activities := map[string]any{
		"start": map[string]any{
			"Wait": map[string]any{
				"WaitTime":     map[string]any{"WaitFor": "PT24H"},
				"NextActivity": "email-step",
			},
		},
		"email-step": map[string]any{
			"EMAIL": map[string]any{
				"MessageConfig":   map[string]any{"FromAddress": "no-reply@example.com"},
				"TemplateName":    "welcome",
				"TemplateVersion": "1",
				"NextActivity":    "holdout",
			},
		},
		"holdout": map[string]any{
			"Holdout": map[string]any{
				"NextActivity": "",
				"Percentage":   15,
			},
		},
	}

	createRec := doPinpointRequest(t, h, http.MethodPost, "/v1/apps/"+appID+"/journeys",
		map[string]any{
			"Name":          "multi-step",
			"StartActivity": "start",
			"Activities":    activities,
		})
	require.Equal(t, http.StatusCreated, createRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &resp))

	returnedActivities, _ := resp["Activities"].(map[string]any)
	assert.Len(t, returnedActivities, 3, "all 3 activities round-trip")
	assert.Contains(t, returnedActivities, "start")
	assert.Contains(t, returnedActivities, "email-step")
	assert.Contains(t, returnedActivities, "holdout")
	assert.Equal(t, "start", resp["StartActivity"])
}

func TestPagination_Journeys_NextToken(t *testing.T) {
	t.Parallel()

	h := newHandlerForTest(t)
	appID := createTestApp(t, h, "paged-journeys-app")

	for i := range 3 {
		doPinpointRequest(t, h, http.MethodPost,
			"/v1/apps/"+appID+"/journeys",
			map[string]any{"Name": fmt.Sprintf("journey-%02d", i)})
	}

	p1Rec := doPinpointRequest(t, h, http.MethodGet,
		"/v1/apps/"+appID+"/journeys?page-size=2", nil)
	require.Equal(t, http.StatusOK, p1Rec.Code)

	var p1 map[string]any
	require.NoError(t, json.Unmarshal(p1Rec.Body.Bytes(), &p1))

	items1, _ := p1["Item"].([]any)
	assert.Len(t, items1, 2)

	tok, ok := p1["NextToken"].(string)
	require.True(t, ok, "NextToken set when more journeys exist")

	p2Rec := doPinpointRequest(t, h, http.MethodGet,
		"/v1/apps/"+appID+"/journeys?page-size=2&token="+tok, nil)
	var p2 map[string]any
	require.NoError(t, json.Unmarshal(p2Rec.Body.Bytes(), &p2))

	items2, _ := p2["Item"].([]any)
	assert.Len(t, items2, 1, "last page returns remaining journey")
	assert.Nil(t, p2["NextToken"])
}

// TestCoverage_JourneyCRUD covers GetJourney, ListJourneys, UpdateJourney,
// UpdateJourneyState, DeleteJourney, GetJourneyDateRangeKpi,
// GetJourneyExecutionMetrics, GetJourneyExecutionActivityMetrics,
// GetJourneyRuns, GetJourneyRunExecutionMetrics, GetJourneyRunExecutionActivityMetrics.
func TestJourneyCRUD(t *testing.T) {
	t.Parallel()

	h := newHandlerForTest(t)
	appID := createTestApp(t, h, "journey-crud-app")

	// Create journey.
	rec := doPinpointRequest(t, h, http.MethodPost, "/v1/apps/"+appID+"/journeys",
		map[string]any{"Name": "test-journey"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&createResp))
	journeyID, _ := createResp["Id"].(string)
	require.NotEmpty(t, journeyID)

	// GetJourney.
	rec = doPinpointRequest(t, h, http.MethodGet, "/v1/apps/"+appID+"/journeys/"+journeyID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// ListJourneys.
	rec = doPinpointRequest(t, h, http.MethodGet, "/v1/apps/"+appID+"/journeys", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// UpdateJourney.
	rec = doPinpointRequest(t, h, http.MethodPut, "/v1/apps/"+appID+"/journeys/"+journeyID,
		map[string]any{"Name": "updated-journey"})
	assert.Equal(t, http.StatusOK, rec.Code)

	// UpdateJourneyState.
	rec = doPinpointRequest(t, h, http.MethodPut, "/v1/apps/"+appID+"/journeys/"+journeyID+"/state",
		map[string]any{"State": "ACTIVE"})
	assert.Equal(t, http.StatusOK, rec.Code)

	// GetJourneyDateRangeKpi.
	rec = doPinpointRequest(
		t,
		h,
		http.MethodGet,
		"/v1/apps/"+appID+"/journeys/"+journeyID+"/kpis/daterange/test-kpi",
		nil,
	)
	assert.Equal(t, http.StatusOK, rec.Code)

	// GetJourneyExecutionMetrics.
	rec = doPinpointRequest(t, h, http.MethodGet, "/v1/apps/"+appID+"/journeys/"+journeyID+"/execution-metrics", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// GetJourneyExecutionActivityMetrics.
	rec = doPinpointRequest(
		t,
		h,
		http.MethodGet,
		"/v1/apps/"+appID+"/journeys/"+journeyID+"/activities/act-1/execution-metrics",
		nil,
	)
	assert.Equal(t, http.StatusOK, rec.Code)

	// GetJourneyRuns.
	rec = doPinpointRequest(t, h, http.MethodGet, "/v1/apps/"+appID+"/journeys/"+journeyID+"/runs", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// GetJourneyRunExecutionMetrics.
	rec = doPinpointRequest(
		t,
		h,
		http.MethodGet,
		"/v1/apps/"+appID+"/journeys/"+journeyID+"/runs/run-1/execution-metrics",
		nil,
	)
	assert.Equal(t, http.StatusOK, rec.Code)

	// GetJourneyRunExecutionActivityMetrics.
	rec = doPinpointRequest(
		t,
		h,
		http.MethodGet,
		"/v1/apps/"+appID+"/journeys/"+journeyID+"/runs/run-1/activities/act-1/execution-metrics",
		nil,
	)
	assert.Equal(t, http.StatusOK, rec.Code)

	// DeleteJourney.
	rec = doPinpointRequest(t, h, http.MethodDelete, "/v1/apps/"+appID+"/journeys/"+journeyID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCreateJourneyAppNotFound(t *testing.T) {
	t.Parallel()

	h := newHandlerForTest(t)

	rec := doPinpointRequest(t, h, http.MethodPost, "/v1/apps/nonexistent/journeys",
		map[string]any{"Name": "orphan-journey"})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestJourneyARNPresent(t *testing.T) {
	t.Parallel()

	h := newHandlerForTest(t)
	appID := createTestApp(t, h, "journey-arn-app")

	rec := doPinpointRequest(t, h, http.MethodPost, "/v1/apps/"+appID+"/journeys",
		map[string]any{"Name": "arn-journey"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.NotEmpty(t, resp["Arn"])
	assert.Contains(t, resp["Arn"].(string), appID)
}

// ──────────────────────────────────────────────────
// Seed helpers
// ──────────────────────────────────────────────────

func TestHandler_CreateJourney(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       any
		name       string
		wantStatus int
		wantID     bool
	}{
		{
			name:       "creates_journey",
			body:       map[string]any{"Name": "my-journey"},
			wantStatus: http.StatusCreated,
			wantID:     true,
		},
		{
			name:       "rejects_empty_name",
			body:       map[string]any{"Name": ""},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandlerForTest(t)
			appID := createTestApp(t, h, "journey-test-app")

			rec := doPinpointRequest(t, h, http.MethodPost, "/v1/apps/"+appID+"/journeys", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantID {
				var resp map[string]any
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				assert.NotEmpty(t, resp["Id"])
				assert.Equal(t, appID, resp["ApplicationId"])
				assert.Equal(t, "DRAFT", resp["State"])
			}
		})
	}
}

// TestHandler_GetJourneys_DuplicateNames_NoDropOrDupAcrossPages proves GetJourneys loses
// (or repeats) journeys at a page boundary when several journeys in the same app share a
// Name. Journey names have no uniqueness constraint (CreateJourney never checks for an
// existing Name), yet GetJourneys sorts solely by Name with no secondary key, over a
// *store.Table map walk whose iteration order varies between calls; handleListJourneys
// then pages that resort with an offset cursor (applyPageParams). Looped since this
// depends on map iteration reshuffling a tie group between the calls backing page 1 and
// page 2, which does not reproduce on every run.
func TestHandler_GetJourneys_DuplicateNames_NoDropOrDupAcrossPages(t *testing.T) {
	t.Parallel()

	for range 30 {
		h := newHandlerForTest(t)
		appID := createTestApp(t, h, "journey-pg-tie-app")

		const dupCount = 5
		created := make(map[string]bool, dupCount)

		for range dupCount {
			rec := doPinpointRequest(t, h, http.MethodPost, "/v1/apps/"+appID+"/journeys",
				map[string]any{"Name": "dup-journey-name"})
			require.Equal(t, http.StatusCreated, rec.Code)

			var resp map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			created[resp["Id"].(string)] = true
		}

		seen := make(map[string]bool, dupCount)
		path := "/v1/apps/" + appID + "/journeys?page-size=2"

		for range dupCount + 1 {
			rec := doPinpointRequest(t, h, http.MethodGet, path, nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

			items, _ := resp["Item"].([]any)
			for _, item := range items {
				j, isMap := item.(map[string]any)
				require.True(t, isMap)
				seen[j["Id"].(string)] = true
			}

			nextToken, hasToken := resp["NextToken"].(string)
			if !hasToken {
				break
			}

			path = "/v1/apps/" + appID + "/journeys?page-size=2&token=" + url.QueryEscape(nextToken)
		}

		assert.Equal(t, created, seen, "paged GetJourneys dropped or duplicated same-named journeys across pages")
	}
}
