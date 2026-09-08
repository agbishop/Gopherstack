package emrserverless

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) applicationARN(applicationID string) string {
	return arn.Build("emr-serverless", b.region, b.accountID, "/applications/"+applicationID)
}

// CreateApplication creates a new EMR Serverless application. If opts carries
// a non-empty ClientToken that was already used successfully, the previously
// created application is returned instead of erroring or creating a
// duplicate -- matching AWS's client-idempotency-token contract, which real
// SDKs rely on when retrying a CreateApplication call after a timeout.
func (b *InMemoryBackend) CreateApplication(
	name, appType, releaseLabel, architecture string,
	tags map[string]string,
	opts ...CreateApplicationOptions,
) (*Application, error) {
	b.mu.Lock("CreateApplication")
	defer b.mu.Unlock()

	var opt CreateApplicationOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}

	if appType == "" {
		return nil, fmt.Errorf("%w: type is required", ErrValidation)
	}

	if opt.ClientToken != "" {
		if appID, tokenOK := b.applicationTokens[opt.ClientToken]; tokenOK {
			if app, appOK := b.applications.Get(appID); appOK {
				return cloneApplication(app), nil
			}
		}
	}

	for _, app := range b.applications.All() {
		if app.Name == name {
			return nil, fmt.Errorf("%w: application %s already exists", ErrAlreadyExists, name)
		}
	}

	id := newID()
	now := time.Now().UTC()

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	app := &Application{
		ApplicationID: id,
		Arn:           b.applicationARN(id),
		Name:          name,
		Type:          appType,
		ReleaseLabel:  releaseLabel,
		Architecture:  architecture,
		State:         ApplicationStateCreated,
		CreatedAt:     now,
		UpdatedAt:     now,
		Tags:          tagsCopy,
		ExtraConfig:   cloneExtraConfig(opt.ExtraConfig),
	}
	b.applications.Put(app)

	if opt.ClientToken != "" {
		b.applicationTokens[opt.ClientToken] = id
	}

	return cloneApplication(app), nil
}

// GetApplication retrieves an application by ID.
func (b *InMemoryBackend) GetApplication(id string) (*Application, error) {
	b.mu.RLock("GetApplication")
	defer b.mu.RUnlock()

	app, ok := b.applications.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: application %s not found", ErrNotFound, id)
	}

	return cloneApplication(app), nil
}

// ListApplications returns paginated applications, optionally filtered by state.
func (b *InMemoryBackend) ListApplications(
	nextToken string, maxResults int, states ...string,
) ([]*Application, string) {
	b.mu.RLock("ListApplications")
	defer b.mu.RUnlock()

	all := b.applications.All()
	list := make([]*Application, 0, len(all))

	for _, app := range all {
		list = append(list, cloneApplication(app))
	}

	if len(states) > 0 {
		stateSet := make(map[string]struct{}, len(states))
		for _, s := range states {
			stateSet[s] = struct{}{}
		}
		filtered := list[:0]
		for _, app := range list {
			if _, ok := stateSet[app.State]; ok {
				filtered = append(filtered, app)
			}
		}
		list = filtered
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].ApplicationID < list[j].ApplicationID
		}

		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})

	page, token := emrPaginate(list, nextToken, maxResults)

	return page, token
}

// UpdateApplication applies a mutating function to an application.
func (b *InMemoryBackend) UpdateApplication(id string, update func(*Application)) (*Application, error) {
	b.mu.Lock("UpdateApplication")
	defer b.mu.Unlock()

	app, ok := b.applications.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: application %s not found", ErrNotFound, id)
	}

	update(app)
	app.UpdatedAt = time.Now().UTC()

	return cloneApplication(app), nil
}

// DeleteApplication removes an application.
// It rejects the request if the application is in STARTED or STARTING state.
func (b *InMemoryBackend) DeleteApplication(id string) error {
	b.mu.Lock("DeleteApplication")
	defer b.mu.Unlock()

	app, ok := b.applications.Get(id)
	if !ok {
		return fmt.Errorf("%w: application %s not found", ErrNotFound, id)
	}

	switch app.State {
	case ApplicationStateStarted, ApplicationStateStarting, ApplicationStateStopping, ApplicationStateCreating:
		return fmt.Errorf(
			"%w: application %s must be stopped before deletion (current state: %s)",
			ErrInvalidState, id, app.State,
		)
	}

	// Cascade-delete this application's job runs and sessions. Clone the
	// index result first: deleting from the table mutates the same backing
	// slice the index lookup returned (see store.Index.Get's ownership note).
	for _, jr := range slices.Clone(b.jobRunsByApplication.Get(id)) {
		b.jobRuns.Delete(jr.JobRunID)
	}
	for _, session := range slices.Clone(b.sessionsByApplication.Get(id)) {
		b.sessions.Delete(session.SessionID)
	}

	b.applications.Delete(id)
	delete(b.sessionTokens, id)
	delete(b.jobRunTokens, id)

	return nil
}

// StartApplication transitions an application to STARTED state.
func (b *InMemoryBackend) StartApplication(id string) error {
	b.mu.Lock("StartApplication")
	defer b.mu.Unlock()

	app, ok := b.applications.Get(id)
	if !ok {
		return fmt.Errorf("%w: application %s not found", ErrNotFound, id)
	}

	switch app.State {
	case ApplicationStateStarted:
		return fmt.Errorf("%w: application %s is already in STARTED state", ErrInvalidState, id)
	case ApplicationStateTerminated:
		return fmt.Errorf(
			"%w: application %s cannot be started from state %s",
			ErrInvalidState, id, app.State,
		)
	case ApplicationStateStarting, ApplicationStateStopping:
		return fmt.Errorf(
			"%w: application %s cannot be started from state %s",
			ErrInvalidState, id, app.State,
		)
	}

	app.State = ApplicationStateStarted
	app.UpdatedAt = time.Now().UTC()

	return nil
}

// StopApplication transitions an application to STOPPED state. All of the
// application's job runs must already be completed or cancelled: "All
// scheduled and running jobs must be completed or cancelled before stopping
// an application" (aws-sdk-go-v2 api_op_StopApplication.go doc comment).
func (b *InMemoryBackend) StopApplication(id string) error {
	b.mu.Lock("StopApplication")
	defer b.mu.Unlock()

	app, ok := b.applications.Get(id)
	if !ok {
		return fmt.Errorf("%w: application %s not found", ErrNotFound, id)
	}

	switch app.State {
	case ApplicationStateStopped, ApplicationStateTerminated:
		return fmt.Errorf("%w: application %s is already in %s state", ErrInvalidState, id, app.State)
	}

	for _, jr := range b.jobRunsByApplication.Get(id) {
		if !isTerminalJobRunState(jr.State) {
			return fmt.Errorf(
				"%w: application %s has job run %s in state %s; all job runs must be completed or cancelled before stopping",
				ErrInvalidState,
				id,
				jr.JobRunID,
				jr.State,
			)
		}
	}

	app.State = ApplicationStateStopped
	app.UpdatedAt = time.Now().UTC()

	return nil
}

// applicationAutoStartEnabled reports whether app's autoStartConfiguration
// permits an implicit start on job/session submission.
// types.AutoStartConfig.Enabled: "Enables the application to automatically
// start on job submission. Defaults to true." -- so absence of the
// sub-object, or of its enabled key, means auto-start is on; only an
// explicit `"enabled": false` turns it off.
func applicationAutoStartEnabled(app *Application) bool {
	raw, ok := app.ExtraConfig["autoStartConfiguration"]
	if !ok {
		return true
	}

	cfg, ok := raw.(map[string]any)
	if !ok {
		return true
	}

	enabled, ok := cfg["enabled"].(bool)
	if !ok {
		return true
	}

	return enabled
}

const (
	// autoStopIdleTimeoutMinutesMin / autoStopIdleTimeoutMinutesMax bound
	// AutoStopConfig.IdleTimeoutMinutes. The Go SDK's doc comment gives only
	// the default of 15 minutes; the range lives in the wire model
	// (botocore emr-serverless/2021-07-13/service-2.json,
	// AutoStopConfigIdleTimeoutMinutesInteger: {"min": 1, "max": 10080}).
	autoStopIdleTimeoutMinutesMin = 1
	autoStopIdleTimeoutMinutesMax = 10080
)

// validateAutoStopConfig checks extra's autoStopConfiguration.idleTimeoutMinutes,
// if present, against the documented AWS range. Absence of the sub-object or
// key is valid -- idleTimeoutMinutes is optional and defaults to 15.
func validateAutoStopConfig(extra map[string]any) error {
	raw, ok := extra["autoStopConfiguration"]
	if !ok {
		return nil
	}

	cfg, ok := raw.(map[string]any)
	if !ok {
		return nil
	}

	val, ok := cfg["idleTimeoutMinutes"]
	if !ok {
		return nil
	}

	n, ok := val.(float64)
	if !ok {
		return nil
	}

	if n < autoStopIdleTimeoutMinutesMin || n > autoStopIdleTimeoutMinutesMax {
		return fmt.Errorf(
			"%w: autoStopConfiguration.idleTimeoutMinutes must be between %d and %d",
			ErrValidation, autoStopIdleTimeoutMinutesMin, autoStopIdleTimeoutMinutesMax,
		)
	}

	return nil
}

// cloneApplication returns a deep copy of an Application with its Tags map cloned.
// The returned copy always has a non-nil Tags map.
func cloneApplication(app *Application) *Application {
	cp := *app
	cp.Tags = make(map[string]string, len(app.Tags))
	maps.Copy(cp.Tags, app.Tags)
	cp.ExtraConfig = cloneExtraConfig(app.ExtraConfig)

	return &cp
}

// cloneExtraConfig returns a shallow copy of an Application's pass-through
// configuration map (see Application.ExtraConfig), with each entry itself
// shallow-cloned via cloneJSONValue.
func cloneExtraConfig(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = cloneJSONValue(v)
	}

	return cp
}

// AddApplicationInternal directly inserts an Application into the backend without
// going through the HTTP layer.  Intended for test seeding only.
func (b *InMemoryBackend) AddApplicationInternal(app *Application) {
	b.mu.Lock("AddApplicationInternal")
	defer b.mu.Unlock()

	if app.Tags == nil {
		app.Tags = make(map[string]string)
	}

	b.applications.Put(app)
}
