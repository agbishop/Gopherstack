package pinpoint

import (
	"fmt"
	"slices"
	"sort"
	"strconv"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// AddJourneyInternal seeds a journey directly without going through the HTTP layer.
func (b *InMemoryBackend) AddJourneyInternal(j *Journey) {
	b.mu.Lock("AddJourneyInternal")
	defer b.mu.Unlock()

	b.journeys.Put(j)
	b.arnIndex[j.ARN] = j
}

// CreateJourney creates a new Pinpoint journey for an application.
func (b *InMemoryBackend) CreateJourney(region, accountID, appID string, req createJourneyRequest) (*Journey, error) {
	b.mu.Lock("CreateJourney")
	defer b.mu.Unlock()

	if _, ok := b.apps.Get(appID); !ok {
		return nil, ErrAppNotFound
	}

	id := uuid.NewString()
	journeyARN := arn.Build("mobiletargeting", region, accountID, fmt.Sprintf("apps/%s/journeys/%s", appID, id))
	now := nowRFC3339()

	j := &Journey{
		ApplicationID:          appID,
		ARN:                    journeyARN,
		ID:                     id,
		Name:                   req.Name,
		State:                  journeyStateDraft,
		StartActivity:          req.StartActivity,
		RefreshFrequency:       req.RefreshFrequency,
		LocalTime:              req.LocalTime,
		WaitForQuietTime:       req.WaitForQuietTime,
		RefreshOnSegmentUpdate: req.RefreshOnSegmentUpdate,
		StartCondition:         cloneAnyMap(req.StartCondition),
		Schedule:               cloneAnyMap(req.Schedule),
		Limits:                 cloneAnyMap(req.Limits),
		QuietTime:              cloneAnyMap(req.QuietTime),
		OpenHours:              cloneAnyMap(req.OpenHours),
		ClosedDays:             cloneAnyMap(req.ClosedDays),
		CreationDate:           now,
		LastModifiedDate:       now,
	}

	if req.Activities != nil {
		j.Activities = make(map[string]map[string]any, len(req.Activities))
		for k, v := range req.Activities {
			j.Activities[k] = cloneAnyMap(v)
		}
	}

	b.journeys.Put(j)
	b.arnIndex[journeyARN] = j

	return cloneJourney(j), nil
}

func cloneJourney(j *Journey) *Journey {
	cp := *j
	cp.Tags = nonNilTagsCopy(j.Tags)
	cp.StartCondition = cloneAnyMap(j.StartCondition)
	cp.Schedule = cloneAnyMap(j.Schedule)
	cp.Limits = cloneAnyMap(j.Limits)
	cp.QuietTime = cloneAnyMap(j.QuietTime)
	cp.OpenHours = cloneAnyMap(j.OpenHours)
	cp.ClosedDays = cloneAnyMap(j.ClosedDays)

	if j.Activities != nil {
		cp.Activities = make(map[string]map[string]any, len(j.Activities))
		for k, v := range j.Activities {
			cp.Activities[k] = cloneAnyMap(v)
		}
	}

	return &cp
}

// GetJourney retrieves a Pinpoint journey by appID and journeyID.
func (b *InMemoryBackend) GetJourney(appID, journeyID string) (*Journey, error) {
	b.mu.RLock("GetJourney")
	defer b.mu.RUnlock()

	j, ok := b.journeys.Get(journeyID)
	if !ok || j.ApplicationID != appID {
		return nil, ErrAppNotFound
	}

	return cloneJourney(j), nil
}

// GetJourneys returns all journeys for an application.
func (b *InMemoryBackend) GetJourneys(appID string) ([]*Journey, error) {
	b.mu.RLock("GetJourneys")
	defer b.mu.RUnlock()

	if _, ok := b.apps.Get(appID); !ok {
		return nil, ErrAppNotFound
	}

	var journeys []*Journey

	for _, j := range b.journeys.All() {
		if j.ApplicationID == appID {
			journeys = append(journeys, cloneJourney(j))
		}
	}

	sort.Slice(journeys, func(i, j int) bool {
		if journeys[i].Name != journeys[j].Name {
			return journeys[i].Name < journeys[j].Name
		}

		return journeys[i].ID < journeys[j].ID
	})

	return journeys, nil
}

// UpdateJourney updates an existing Pinpoint journey.
func (b *InMemoryBackend) UpdateJourney(
	appID, journeyID string,
	req updateJourneyRequest,
) (*Journey, error) {
	b.mu.Lock("UpdateJourney")
	defer b.mu.Unlock()

	j, ok := b.journeys.Get(journeyID)
	if !ok || j.ApplicationID != appID {
		return nil, ErrAppNotFound
	}

	if j.State == journeyStateActive {
		return nil, ErrJourneyActive
	}

	if req.Name != "" {
		j.Name = req.Name
	}

	if req.StartActivity != "" {
		j.StartActivity = req.StartActivity
	}

	if req.RefreshFrequency != "" {
		j.RefreshFrequency = req.RefreshFrequency
	}

	if len(req.Activities) > 0 {
		j.Activities = make(map[string]map[string]any, len(req.Activities))
		for k, v := range req.Activities {
			j.Activities[k] = cloneAnyMap(v)
		}
	}

	if len(req.StartCondition) > 0 {
		j.StartCondition = cloneAnyMap(req.StartCondition)
	}

	if len(req.Schedule) > 0 {
		j.Schedule = cloneAnyMap(req.Schedule)
	}

	if len(req.Limits) > 0 {
		j.Limits = cloneAnyMap(req.Limits)
	}

	if len(req.QuietTime) > 0 {
		j.QuietTime = cloneAnyMap(req.QuietTime)
	}

	if len(req.OpenHours) > 0 {
		j.OpenHours = cloneAnyMap(req.OpenHours)
	}

	if len(req.ClosedDays) > 0 {
		j.ClosedDays = cloneAnyMap(req.ClosedDays)
	}

	j.LocalTime = req.LocalTime
	j.WaitForQuietTime = req.WaitForQuietTime
	j.RefreshOnSegmentUpdate = req.RefreshOnSegmentUpdate
	j.LastModifiedDate = nowRFC3339()

	return cloneJourney(j), nil
}

// allowedJourneyTransitions returns the set of valid target states for a given source state.
func allowedJourneyTransitions(from string) ([]string, bool) {
	transitions := map[string][]string{
		journeyStateDraft:     {journeyStateActive, journeyStateCancelled},
		journeyStateActive:    {journeyStatePaused, journeyStateCancelled, journeyStateCompleted},
		journeyStatePaused:    {journeyStateActive, journeyStateCancelled},
		journeyStateCancelled: {},
		journeyStateCompleted: {},
		journeyStateClosed:    {},
	}

	allowed, ok := transitions[from]

	return allowed, ok
}

// UpdateJourneyState updates the state of a Pinpoint journey.
func (b *InMemoryBackend) UpdateJourneyState(appID, journeyID, state string) (*Journey, error) {
	b.mu.Lock("UpdateJourneyState")
	defer b.mu.Unlock()

	j, ok := b.journeys.Get(journeyID)
	if !ok || j.ApplicationID != appID {
		return nil, ErrAppNotFound
	}

	allowed, exists := allowedJourneyTransitions(j.State)
	if !exists {
		return nil, ErrValidation
	}

	if !slices.Contains(allowed, state) {
		return nil, ErrValidation
	}

	j.State = state
	j.LastModifiedDate = nowRFC3339()

	if state == journeyStateActive {
		runNow := nowRFC3339()
		runKey := appID + "/" + journeyID
		b.journeyRuns[runKey] = append(b.journeyRuns[runKey], &journeyRun{
			RunID:          uuid.NewString(),
			JourneyID:      journeyID,
			ApplicationID:  appID,
			Status:         "SCHEDULED",
			CreationTime:   runNow,
			LastUpdateTime: runNow,
		})
	}

	return cloneJourney(j), nil
}

// DeleteJourney deletes a Pinpoint journey.
func (b *InMemoryBackend) DeleteJourney(appID, journeyID string) (*Journey, error) {
	b.mu.Lock("DeleteJourney")
	defer b.mu.Unlock()

	j, ok := b.journeys.Get(journeyID)
	if !ok || j.ApplicationID != appID {
		return nil, ErrAppNotFound
	}

	b.journeys.Delete(journeyID)
	delete(b.arnIndex, j.ARN)
	delete(b.journeyRuns, appID+"/"+journeyID)

	return cloneJourney(j), nil
}

// GetJourneyDateRangeKpi returns stub KPI data for a journey.
func (b *InMemoryBackend) GetJourneyDateRangeKpi(
	appID, journeyID, kpiName, startTime, endTime string,
) (*kpiResult, error) {
	b.mu.RLock("GetJourneyDateRangeKpi")
	defer b.mu.RUnlock()

	j, ok := b.journeys.Get(journeyID)
	if !ok || j.ApplicationID != appID {
		return nil, ErrAppNotFound
	}

	return &kpiResult{
		ApplicationID: appID,
		JourneyID:     journeyID,
		KpiName:       kpiName,
		StartTime:     startTime,
		EndTime:       endTime,
		KpiResult:     kpiRows{Rows: []kpiRow{}},
	}, nil
}

// GetJourneyExecutionMetrics returns journey execution metrics.
func (b *InMemoryBackend) GetJourneyExecutionMetrics(
	appID, journeyID string,
) (*journeyExecutionMetricsResponse, error) {
	b.mu.RLock("GetJourneyExecutionMetrics")
	defer b.mu.RUnlock()

	j, ok := b.journeys.Get(journeyID)
	if !ok || j.ApplicationID != appID {
		return nil, ErrAppNotFound
	}

	runKey := appID + "/" + journeyID
	runs := b.journeyRuns[runKey]

	return &journeyExecutionMetricsResponse{
		ApplicationID: appID,
		JourneyID:     journeyID,
		Metrics: map[string]string{
			"TotalRuns": strconv.Itoa(len(runs)),
		},
		LastEvaluatedTime: nowRFC3339(),
	}, nil
}

// GetJourneyExecutionActivityMetrics returns journey activity metrics.
func (b *InMemoryBackend) GetJourneyExecutionActivityMetrics(
	appID, journeyID, activityID string,
) (*journeyExecutionActivityMetricsResponse, error) {
	b.mu.RLock("GetJourneyExecutionActivityMetrics")
	defer b.mu.RUnlock()

	j, ok := b.journeys.Get(journeyID)
	if !ok || j.ApplicationID != appID {
		return nil, ErrAppNotFound
	}

	runKey := appID + "/" + journeyID
	runs := b.journeyRuns[runKey]

	return &journeyExecutionActivityMetricsResponse{
		ApplicationID: appID,
		JourneyID:     journeyID,
		ActivityID:    activityID,
		Metrics: map[string]string{
			"TotalRuns":  strconv.Itoa(len(runs)),
			"ActivityId": activityID,
		},
		LastEvaluatedTime: nowRFC3339(),
	}, nil
}

// GetJourneyRuns returns journey runs.
func (b *InMemoryBackend) GetJourneyRuns(appID, journeyID string) (*journeyRunsResponse, error) {
	b.mu.RLock("GetJourneyRuns")
	defer b.mu.RUnlock()

	j, ok := b.journeys.Get(journeyID)
	if !ok || j.ApplicationID != appID {
		return nil, ErrAppNotFound
	}

	runKey := appID + "/" + journeyID
	storedRuns := b.journeyRuns[runKey]

	runs := make([]journeyRun, 0, len(storedRuns))
	for _, r := range storedRuns {
		runs = append(runs, *r)
	}

	return &journeyRunsResponse{Item: runs}, nil
}

// GetJourneyRunExecutionMetrics returns metrics for a specific journey run.
func (b *InMemoryBackend) GetJourneyRunExecutionMetrics(
	appID, journeyID, runID string,
) (*journeyRunExecutionMetricsResponse, error) {
	b.mu.RLock("GetJourneyRunExecutionMetrics")
	defer b.mu.RUnlock()

	j, ok := b.journeys.Get(journeyID)
	if !ok || j.ApplicationID != appID {
		return nil, ErrAppNotFound
	}

	return &journeyRunExecutionMetricsResponse{
		ApplicationID: appID,
		JourneyID:     journeyID,
		RunID:         runID,
		Metrics: map[string]string{
			"RunId": runID,
		},
		LastEvaluatedTime: nowRFC3339(),
	}, nil
}

// GetJourneyRunExecutionActivityMetrics returns metrics for a specific journey run activity.
func (b *InMemoryBackend) GetJourneyRunExecutionActivityMetrics(
	appID, journeyID, runID, activityID string,
) (*journeyRunExecutionActivityMetricsResponse, error) {
	b.mu.RLock("GetJourneyRunExecutionActivityMetrics")
	defer b.mu.RUnlock()

	j, ok := b.journeys.Get(journeyID)
	if !ok || j.ApplicationID != appID {
		return nil, ErrAppNotFound
	}

	return &journeyRunExecutionActivityMetricsResponse{
		ApplicationID: appID,
		JourneyID:     journeyID,
		RunID:         runID,
		ActivityID:    activityID,
		Metrics: map[string]string{
			"RunId":      runID,
			"ActivityId": activityID,
		},
		LastEvaluatedTime: nowRFC3339(),
	}, nil
}
